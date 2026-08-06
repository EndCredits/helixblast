package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/EndCredits/helixblast/internal/blast"
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
