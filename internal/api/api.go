package api

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"helixblast/embed"
	"helixblast/internal/config"
	custommw "helixblast/internal/middleware"
	"helixblast/internal/transcript"
	"helixblast/internal/worker"
)

type Server struct {
	router  chi.Router
	pool    *worker.Pool
	dbMgr   *config.DatabaseManager
	config  *config.Config
	version string
}

func NewServer(cfg *config.Config, pool *worker.Pool, dbMgr *config.DatabaseManager) *Server {
	s := &Server{
		router:  chi.NewRouter(),
		pool:    pool,
		dbMgr:   dbMgr,
		config:  cfg,
		version: "0.1.0",
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

	s.router.Route("/api/v1/jobs", func(r chi.Router) {
		r.Post("/", s.handleJobCreate)
		r.Get("/", s.handleJobList)
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
		Name        string `json:"name"`
		Type        string `json:"type"`
		Description string `json:"description"`
		LastUpdated string `json:"last_updated"`
	}

	out := make([]dbResponse, len(dbs))
	for i, db := range dbs {
		out[i] = dbResponse{
			Name:        db.Name,
			Type:        db.Type,
			Description: db.Description,
			LastUpdated: db.LastUpdated,
		}
	}

	jsonResponse(w, http.StatusOK, out)
}

func (s *Server) handleJobCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FastA         string            `json:"fasta"`
		Program       string            `json:"program"`
		Database      string            `json:"db"`
		Template      string            `json:"template,omitempty"`
		AdvancedParams map[string]string `json:"advanced_params,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.FastA == "" || req.Program == "" || req.Database == "" {
		jsonError(w, http.StatusBadRequest, "fasta, program, and db are required")
		return
	}

	job := worker.NewJob(req.Program, req.Database, req.FastA, req.AdvancedParams)

	if err := s.pool.Submit(job); err != nil {
		jsonError(w, http.StatusTooManyRequests, err.Error())
		return
	}

	jsonResponse(w, http.StatusCreated, map[string]any{
		"job_id":    job.ID,
		"status":    string(job.GetStatus()),
		"queue_pos": job.QueuePos,
	})
}

func (s *Server) handleJobList(w http.ResponseWriter, r *http.Request) {
	jobs := s.pool.List()

	type jobItem struct {
		JobID     string    `json:"job_id"`
		Status    string    `json:"status"`
		QueuePos  int       `json:"queue_pos"`
		Program   string    `json:"program"`
		Database  string    `json:"database"`
		CreatedAt time.Time `json:"created_at"`
	}

	out := make([]jobItem, len(jobs))
	for i, j := range jobs {
		out[i] = jobItem{
			JobID:     j.ID,
			Status:    string(j.Status),
			QueuePos:  j.QueuePos,
			Program:   j.Program,
			Database:  j.Database,
			CreatedAt: j.CreatedAt,
		}
	}

	if out == nil {
		out = []jobItem{}
	}

	jsonResponse(w, http.StatusOK, out)
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
}

func (s *Server) handleJobCancel(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	if err := s.pool.Cancel(jobID); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusAccepted)
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
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-subCh:
			if !send() {
				return
			}
		}
	}
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
		seq, err := s.fetchSequenceFromWorker(result)
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

func (s *Server) fetchSequenceFromWorker(res *transcript.Result) (string, error) {
	scanStart := res.Start - 5000
	if scanStart < 1 {
		scanStart = 1
	}

	u, err := url.Parse(fmt.Sprintf("%s/sequence", s.config.Database.WorkerURL))
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("db", res.Database)
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
	gffData, err := transcript.LoadIndex(dbEntry.Transcript.IndexPath)
	if err != nil {
		return nil, fmt.Errorf("load index: %w", err)
	}

	result, err := transcript.Lookup(
		gffData,
		dbEntry.Name,
		transcriptID,
		dbEntry.Transcript.FastaDir,
		dbEntry.Transcript.FastaFile,
	)
	if err != nil {
		return nil, err
	}

	return result, nil
}
