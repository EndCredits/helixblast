package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeJSONIndex(t *testing.T, path string, data *GFF3Data) {
	t.Helper()
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func smallIndex(geneID string) *GFF3Data {
	return &GFF3Data{
		Entries: GFF3Index{
			geneID: {Chr: "Chr01", Start: 100, End: 200, Strand: "+", Type: "gene", Gene: geneID},
		},
		Families:   GFF3Families{geneID: {Transcripts: []string{geneID}}},
		Coords:     GFF3Coords{},
		FastaIndex: map[string]int64{"Chr01": 0},
		Spatial:    GFF3Spatial{},
	}
}

func TestJSONCache_Hit(t *testing.T) {
	invalidateJSONCache()
	path := filepath.Join(t.TempDir(), "idx.json")
	writeJSONIndex(t, path, smallIndex("g1"))

	r1, err := LoadIndexAuto(path)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := LoadIndexAuto(path)
	if err != nil {
		t.Fatal(err)
	}
	// Same path, unchanged file → identical cached reader (pointer equality).
	if r1 != r2 {
		t.Error("expected cached reader to be reused across calls")
	}
	// Close is a no-op and must not evict the shared reader.
	if err := r1.Close(); err != nil {
		t.Fatal(err)
	}
	r3, _ := LoadIndexAuto(path)
	if r3 != r1 {
		t.Error("Close() must not invalidate the shared JSON reader")
	}
}

func TestJSONCache_InvalidatesOnFileChange(t *testing.T) {
	invalidateJSONCache()
	path := filepath.Join(t.TempDir(), "idx.json")
	writeJSONIndex(t, path, smallIndex("old_gene"))

	r1, _ := LoadIndexAuto(path)
	if _, ok := r1.LookupEntry("old_gene"); !ok {
		t.Fatal("precondition: old_gene missing")
	}

	// Rewrite with different content and size; bump mtime forward to defeat
	// coarse filesystem timestamp granularity.
	writeJSONIndex(t, path, smallIndex("a_much_longer_gene_identifier_here"))
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}

	r2, err := LoadIndexAuto(path)
	if err != nil {
		t.Fatal(err)
	}
	if r2 == r1 {
		t.Error("reader should be reloaded after file change")
	}
	if _, ok := r2.LookupEntry("a_much_longer_gene_identifier_here"); !ok {
		t.Error("reloaded reader missing new entry")
	}
	if _, ok := r2.LookupEntry("old_gene"); ok {
		t.Error("reloaded reader still holds stale entry")
	}
}

func TestJSONCache_PerPathIsolation(t *testing.T) {
	invalidateJSONCache()
	dir := t.TempDir()
	pa := filepath.Join(dir, "a.json")
	pb := filepath.Join(dir, "b.json")
	writeJSONIndex(t, pa, smallIndex("gene_a"))
	writeJSONIndex(t, pb, smallIndex("gene_b"))

	ra, _ := LoadIndexAuto(pa)
	rb, _ := LoadIndexAuto(pb)
	if ra == rb {
		t.Error("different paths must not share a reader")
	}
	if _, ok := ra.LookupEntry("gene_a"); !ok {
		t.Error("path a reader missing gene_a")
	}
	if _, ok := rb.LookupEntry("gene_b"); !ok {
		t.Error("path b reader missing gene_b")
	}
	if _, ok := ra.LookupEntry("gene_b"); ok {
		t.Error("path a reader must not see gene_b")
	}
}
