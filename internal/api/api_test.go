package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EndCredits/helixblast/internal/blast"
	"github.com/EndCredits/helixblast/internal/config"
	"github.com/EndCredits/helixblast/internal/worker"
)

func TestHandleJobCreateRejectsUnknownParam(t *testing.T) {
	wl := blast.NewParamWhitelist([]string{"task", "evalue", "word_size"})
	s := &Server{whitelist: wl}

	body := `{"fasta":">seq1\nATGCGTAC","program":"blastn","db":"nt","advanced_params":{"bogus_param":"1"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleJobCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown param, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(resp["error"], "invalid blast parameter: -bogus_param") {
		t.Fatalf("unexpected error message: %q", resp["error"])
	}
}

func TestHandleJobCreateRejectsUnknownParamFromTemplateTask(t *testing.T) {
	// template maps to advanced_params.task, which must be whitelisted.
	wl := blast.NewParamWhitelist([]string{"evalue"}) // task intentionally missing
	s := &Server{whitelist: wl}

	body := `{"fasta":">seq1\nATGCGTAC","program":"blastn","db":"nt","template":"megablast"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleJobCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for template->task not in whitelist, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(resp["error"], "invalid blast parameter: -task") {
		t.Fatalf("unexpected error message: %q", resp["error"])
	}
}

func TestHandleJobCreateAllowsWhitelistedParams(t *testing.T) {
	// Only advanced_params is checked before the database lookup. A valid
	// param must pass the whitelist check (and then fail at db lookup since
	// this test has no DatabaseManager — but NOT with an "invalid blast parameter").
	wl := blast.NewParamWhitelist([]string{"task", "evalue"})
	s := &Server{whitelist: wl}

	body := `{"fasta":">seq1\nATGCGTAC","program":"blastn","db":"nt","advanced_params":{"evalue":"1e-10","task":"megablast"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
	w := httptest.NewRecorder()

	func() {
		defer func() {
			if r := recover(); r != nil {
				// dbMgr is nil in this test; a panic after whitelist pass is
				// acceptable proof the params were not rejected.
				t.Logf("whitelist passed; later nil-dbMgr panic expected: %v", r)
			}
		}()
		s.handleJobCreate(w, req)
	}()

	if strings.Contains(w.Body.String(), "invalid blast parameter") {
		t.Fatalf("whitelisted params were rejected: %s", w.Body.String())
	}
}

func TestHandleJobCancelUnknownIDReturns404(t *testing.T) {
	pool := worker.NewPool(1, 1, nil, time.Hour)
	defer pool.Stop()
	s := &Server{pool: pool}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/jobs/hxb-00000000", nil)
	w := httptest.NewRecorder()
	s.handleJobCancel(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown job id, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(resp["error"], "job not found") {
		t.Fatalf("unexpected error message: %q", resp["error"])
	}
}

func TestHandleJobCreateDuringShutdownReturns503(t *testing.T) {
	dir := t.TempDir()
	dbFile := filepath.Join(dir, "databases.yaml")
	if err := os.WriteFile(dbFile, []byte("databases:\n  - name: nt\n    type: nucleotide\n    path: /tmp/nt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dm, err := config.NewDatabaseManager(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer dm.Stop()

	pool := worker.NewPool(1, 1, nil, time.Hour)
	pool.Stop()

	s := &Server{pool: pool, dbMgr: dm, whitelist: blast.NewParamWhitelist([]string{"task"})}
	body := `{"fasta":">seq1\nATGCGTAC","program":"blastn","db":"nt"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleJobCreate(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 during shutdown, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleJobCreateDedupesDatabases(t *testing.T) {
	dir := t.TempDir()
	dbFile := filepath.Join(dir, "databases.yaml")
	if err := os.WriteFile(dbFile, []byte("databases:\n  - name: nt\n    type: nucleotide\n    path: /tmp/nt\n  - name: nr\n    type: protein\n    path: /tmp/nr\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dm, err := config.NewDatabaseManager(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer dm.Stop()

	pool := worker.NewPool(1, 4, func(ctx context.Context, job *worker.Job, dbName string) ([]blast.Hit, error) {
		return nil, nil
	}, time.Hour)
	defer pool.Stop()
	s := &Server{pool: pool, dbMgr: dm, whitelist: blast.NewParamWhitelist([]string{"task"})}

	body := `{"fasta":">seq1\nATGCGTAC","program":"blastn","dbs":["nt","nr","nt"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleJobCreate(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	job, err := pool.Get(resp.JobID)
	if err != nil {
		t.Fatal(err)
	}
	// Databases is set at construction and only read afterwards — safe to inspect.
	got := len(job.Databases)
	if got != 2 {
		t.Errorf("dbs [nt,nr,nt] should dedupe to 2, got %d: %v", got, job.Databases)
	}
}

func TestHandleSpatialWindowTooLargeReturns400(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/spatial?db=x&chr=c1&start=1&end=2000001", nil)
	w := httptest.NewRecorder()
	s.handleSpatialLookup(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized window, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "too large") {
		t.Errorf("unexpected error body: %s", w.Body.String())
	}
	// reversed bounds of legal width must still pass validation (db lookup
	// then 404s on unknown db — proving the width gate let it through)
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/spatial?db=x&chr=c1&start=5000&end=1", nil)
	w2 := httptest.NewRecorder()
	func() {
		defer func() { recover() }() // nil dbMgr after the gate is expected
		s.handleSpatialLookup(w2, req2)
	}()
	if w2.Code == http.StatusBadRequest && strings.Contains(w2.Body.String(), "too large") {
		t.Error("reversed legal-width window should not be rejected")
	}
}
