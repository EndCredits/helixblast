package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EndCredits/helixblast/internal/config"
	"github.com/EndCredits/helixblast/internal/transcript"
)

func TestHandleTranscriptLookupMinusStrandWorkerWindow(t *testing.T) {
	dir := t.TempDir()
	idx := transcript.GFF3Data{
		Entries: transcript.GFF3Index{
			"tx": transcript.GFF3Entry{
				Chr: "Chr1", Start: 7001, End: 7500, Strand: "-", Type: "mRNA", Gene: "g1",
			},
		},
	}
	raw, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	idxPath := filepath.Join(dir, "idx.json")
	if err := os.WriteFile(idxPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	dbFile := filepath.Join(dir, "databases.yaml")
	dbYAML := fmt.Sprintf(`databases:
  - name: db
    type: nucleotide
    path: /tmp/db
    transcript:
      index_path: %q
      source: src
`, idxPath)
	if err := os.WriteFile(dbFile, []byte(dbYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	dm, err := config.NewDatabaseManager(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer dm.Stop()

	var got url.Values
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"sequence":%q}`, strings.Repeat("A", 3000))
	}))
	defer worker.Close()

	s := &Server{
		dbMgr:  dm,
		config: &config.Config{Database: config.DatabaseConfig{WorkerURL: worker.URL}},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transcripts?db=db&transcript=tx", nil)
	rec := httptest.NewRecorder()
	s.handleTranscriptLookup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got.Get("start") != "7001" || got.Get("end") != "12500" {
		t.Fatalf("worker window = %s-%s, want 7001-12500", got.Get("start"), got.Get("end"))
	}
	var body struct {
		ScanStart int    `json:"scan_start"`
		ScanEnd   int    `json:"scan_end"`
		Sequence  string `json:"sequence"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ScanStart != 7001 || body.ScanEnd != 10000 {
		t.Errorf("scan window = %d-%d, want 7001-10000", body.ScanStart, body.ScanEnd)
	}
	if len(body.Sequence) != 3000 {
		t.Errorf("sequence len = %d, want 3000", len(body.Sequence))
	}
}
