package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/EndCredits/helixblast/internal/blast"
	"github.com/EndCredits/helixblast/embed"
	"github.com/EndCredits/helixblast/internal/config"
	custommw "github.com/EndCredits/helixblast/internal/middleware"
	"github.com/EndCredits/helixblast/internal/transcript"
	"github.com/EndCredits/helixblast/internal/worker"
)

type Server struct {
	router    chi.Router
	pool      *worker.Pool
	dbMgr     *config.DatabaseManager
	config    *config.Config
	whitelist *blast.ParamWhitelist
	version   string
}

func NewServer(cfg *config.Config, pool *worker.Pool, dbMgr *config.DatabaseManager, whitelist *blast.ParamWhitelist) *Server {
	s := &Server{
		router:    chi.NewRouter(),
		pool:      pool,
		dbMgr:     dbMgr,
		config:    cfg,
		whitelist: whitelist,
		version:   "0.1.0",
	}

	s.router.Use(chimw.RequestID)
	s.router.Use(chimw.RealIP)
	s.router.Use(custommw.Logger)
	s.router.Use(chimw.Recoverer)
	s.router.Use(custommw.CORS)
	s.router.Use(custommw.NewRateLimiter(100, 120).Middleware)

	s.router.Get("/health", s.handleHealth)
	s.router.Get("/api/v1/databases", s.handleDatabases)
	s.router.Get("/api/v1/transcripts", s.handleTranscriptLookup)
	s.router.Get("/api/v1/spatial", s.handleSpatialLookup)

	s.router.Route("/api/v1/jobs", func(r chi.Router) {
		r.Post("/", s.handleJobCreate)
		r.Get("/{jobID}", s.handleJobGet)
		r.Delete("/{jobID}", s.handleJobCancel)
		r.Get("/{jobID}/events", s.handleJobEvents)
	})

	staticFS, err := fs.Sub(embedded.Frontend, ".")
	if err != nil {
		log.Printf("[helixblast] Failed to mount static frontend: %v", err)
	} else {
		s.router.Handle("/*", http.FileServer(http.FS(staticFS)))
	}

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) ListenAddr() string {
	return fmt.Sprintf(":%d", s.config.Server.Port)
}

func (s *Server) Run() error {
	addr := s.ListenAddr()
	log.Printf("[helixblast] HTTP server listening on %s", addr)
	return http.ListenAndServe(addr, s.router)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resources := s.config.ComputeResources()
	status := "healthy"
	if resources.Degraded {
		status = "degraded"
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"status":              status,
		"version":             s.version,
		"concurrent_capacity": resources.ActualConcurrent,
		"storage_backend":     s.config.Storage.Type,
	})
}

func (s *Server) handleDatabases(w http.ResponseWriter, r *http.Request) {
	dbs := s.dbMgr.List()
	if dbs == nil {
		dbs = []config.DatabaseEntry{}
	}

	type dbResponse struct {
		Name           string `json:"name"`
		Type           string `json:"type"`
		Description    string `json:"description"`
		LastUpdated    string `json:"last_updated"`
		IsChromosomeDB bool   `json:"is_chromosome_db"`
	}

	out := make([]dbResponse, len(dbs))
	for i, db := range dbs {
		out[i] = dbResponse{
			Name:           db.Name,
			Type:           db.Type,
			Description:    db.Description,
			LastUpdated:    db.LastUpdated,
			IsChromosomeDB: db.IsChromosomeDB,
		}
	}

	jsonResponse(w, http.StatusOK, out)
}

func (s *Server) handleJobCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FastA          string            `json:"fasta"`
		Program        string            `json:"program"`
		Database       string            `json:"db"`
		Databases      []string          `json:"dbs"`
		Template       string            `json:"template,omitempty"`
		AdvancedParams map[string]string `json:"advanced_params,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	dbs := req.Databases
	if len(dbs) == 0 {
		if req.Database != "" {
			dbs = []string{req.Database}
		}
	}

	if req.FastA == "" || req.Program == "" || len(dbs) == 0 {
		jsonError(w, http.StatusBadRequest, "fasta, program, and dbs (or db) are required")
		return
	}

	if err := blast.ValidateFASTA(req.FastA, blast.ProgramType(req.Program)); err != nil {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("invalid fasta: %v", err))
		return
	}

	if req.Template != "" {
		if req.AdvancedParams == nil {
			req.AdvancedParams = make(map[string]string)
		}
		if _, exists := req.AdvancedParams["task"]; !exists {
			req.AdvancedParams["task"] = req.Template
		}
	}

	// Validate advanced params against the BLAST parameter whitelist at
	// submission time. Unknown parameters are rejected with 400 — the job
	// never enters the queue. A nil whitelist (build failure) allows all.
	if s.whitelist != nil {
		for key := range req.AdvancedParams {
			if !s.whitelist.IsAllowed(key) {
				jsonError(w, http.StatusBadRequest, fmt.Sprintf("invalid blast parameter: -%s", key))
				return
			}
		}
	}

	for _, dbName := range dbs {
		if _, err := s.dbMgr.Lookup(dbName); err != nil {
			jsonError(w, http.StatusBadRequest, fmt.Sprintf("unknown database: %s", dbName))
			return
		}
	}

	job := worker.NewJob(req.Program, dbs, req.FastA, req.AdvancedParams)

	if err := s.pool.Submit(job); err != nil {
		jsonError(w, http.StatusTooManyRequests, err.Error())
		return
	}

	snap := job.Snapshot()
	jsonResponse(w, http.StatusCreated, map[string]any{
		"job_id":    snap.ID,
		"status":    string(snap.Status),
		"queue_pos": snap.QueuePos,
	})
}

func (s *Server) handleJobGet(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	job, err := s.pool.Get(jobID)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}

	snap := job.Snapshot()
	jsonResponse(w, http.StatusOK, snap)
	if snap.Status == worker.StatusSuccess || snap.Status == worker.StatusFailed || snap.Status == worker.StatusCancelled {
		job.ClearResult()
	}
}

func (s *Server) handleJobCancel(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	if err := s.pool.Cancel(jobID); err != nil {
		if errors.Is(err, worker.ErrJobNotFound) {
			jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	jsonResponse(w, http.StatusAccepted, map[string]string{"status": "cancelling"})
}

func (s *Server) handleJobEvents(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	job, err := s.pool.Get(jobID)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	subCh := job.Subscribe()
	defer job.Unsubscribe(subCh)

	ctx := r.Context()

	send := func() bool {
		snap := job.Snapshot()
		data, err := json.Marshal(snap)
		if err != nil {
			return false
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		status := snap.Status
		return status != worker.StatusSuccess && status != worker.StatusFailed && status != worker.StatusCancelled
	}

	if !send() {
		job.ClearResult()
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-subCh:
			if !send() {
				job.ClearResult()
				return
			}
		}
	}
}

func (s *Server) handleSpatialLookup(w http.ResponseWriter, r *http.Request) {
	db := r.URL.Query().Get("db")
	chr := r.URL.Query().Get("chr")
	posStr := r.URL.Query().Get("pos")
	if db == "" || chr == "" || posStr == "" {
		jsonError(w, http.StatusBadRequest, "db, chr, and pos are required parameters")
		return
	}

	var posInt int
	if _, err := fmt.Sscanf(posStr, "%d", &posInt); err != nil || posInt < 1 {
		jsonError(w, http.StatusBadRequest, "pos must be a positive integer")
		return
	}

	dbEntry, err := s.dbMgr.Lookup(db)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}

	if dbEntry.Transcript.IndexPath == "" {
		jsonError(w, http.StatusServiceUnavailable, "transcript lookup not configured for this database")
		return
	}

	idx, err := transcript.LoadIndexAuto(dbEntry.Transcript.IndexPath)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("load index: %v", err))
		return
	}
	defer idx.Close()

	result, err := transcript.SpatialLookupV2(idx, chr, posInt)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, result)
}

func jsonResponse(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	jsonResponse(w, code, map[string]string{"error": msg})
}

func (s *Server) handleTranscriptLookup(w http.ResponseWriter, r *http.Request) {
	db := r.URL.Query().Get("db")
	transcriptID := r.URL.Query().Get("transcript")
	if db == "" || transcriptID == "" {
		jsonError(w, http.StatusBadRequest, "db and transcript are required parameters")
		return
	}

	dbEntry, err := s.dbMgr.Lookup(db)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}

	if dbEntry.Transcript.IndexPath == "" {
		jsonError(w, http.StatusServiceUnavailable, "transcript lookup not configured (set transcript.index_path in databases.yaml)")
		return
	}

	result, err := s.localTranscriptLookup(dbEntry, transcriptID)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}

	if result.Sequence == "" && s.config.Database.WorkerURL != "" && dbEntry.Transcript.FastaFile == "" && dbEntry.Transcript.FastaDir == "" {
		seq, err := s.fetchSequenceFromWorker(dbEntry, result)
		if err != nil {
			jsonError(w, http.StatusBadGateway, fmt.Sprintf("worker sequence fetch failed: %v", err))
			return
		}
		result.Sequence = seq
		result.ScanStart = result.Start - 5000
		if result.ScanStart < 1 {
			result.ScanStart = 1
		}
		result.ScanEnd = result.End
	}

	jsonResponse(w, http.StatusOK, result)
}

func (s *Server) fetchSequenceFromWorker(dbEntry *config.DatabaseEntry, res *transcript.Result) (string, error) {
	scanStart := res.Start - 5000
	if scanStart < 1 {
		scanStart = 1
	}

	dbName := dbEntry.Name
	if dbEntry.Transcript.Source != "" {
		dbName = dbEntry.Transcript.Source
	}

	u, err := url.Parse(fmt.Sprintf("%s/sequence", s.config.Database.WorkerURL))
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("db", dbName)
	q.Set("chr", res.Chromosome)
	q.Set("start", fmt.Sprintf("%d", scanStart))
	q.Set("end", fmt.Sprintf("%d", res.End))
	q.Set("strand", res.Strand)
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("worker request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("worker returned %d: %s", resp.StatusCode, string(body))
	}

	var workerResp struct {
		Sequence string `json:"sequence"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&workerResp); err != nil {
		return "", fmt.Errorf("decode worker response: %w", err)
	}

	return workerResp.Sequence, nil
}

func (s *Server) localTranscriptLookup(dbEntry *config.DatabaseEntry, transcriptID string) (*transcript.Result, error) {
	idx, err := transcript.LoadIndexAuto(dbEntry.Transcript.IndexPath)
	if err != nil {
		return nil, fmt.Errorf("load index: %w", err)
	}
	defer idx.Close()

	result, err := transcript.LookupWithIndex(
		idx,
		dbEntry.Name,
		transcriptID,
		dbEntry.Transcript.FastaDir,
		dbEntry.Transcript.FastaFile,
		idx.FastaIndexMap(),
	)
	if err != nil {
		return nil, err
	}

	return result, nil
}
