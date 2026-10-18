package transcript

import (
	"os"
	"path/filepath"
	"testing"
)

func writeStrandFixture(t *testing.T, chr string, length int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, chr+".fa")
	content := ">" + chr + "\n" + fixtureSeq(chr, 1, length) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func strandIndex(entry GFF3Entry) *jsonIndexReader {
	return &jsonIndexReader{data: &GFF3Data{
		Entries: GFF3Index{"tx": entry},
	}}
}

func TestLookupWithIndex_PlusStrandWindow(t *testing.T) {
	const chr = "Chr1"
	dir := writeStrandFixture(t, chr, 12000)
	idx := strandIndex(GFF3Entry{
		Chr: chr, Start: 4001, End: 4500, Strand: "+", Type: "mRNA", Gene: "g1",
	})

	got, err := LookupWithIndex(idx, "db", "tx", dir, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.ScanStart != 1 || got.ScanEnd != 4500 {
		t.Fatalf("scan window = %d-%d, want 1-4500", got.ScanStart, got.ScanEnd)
	}
	if want := fixtureSeq(chr, 1, 4500); got.Sequence != want {
		t.Errorf("sequence mismatch: got len %d, want len %d", len(got.Sequence), len(want))
	}
}

func TestLookupWithIndex_MinusStrandWindow(t *testing.T) {
	const chr = "Chr1"
	dir := writeStrandFixture(t, chr, 12000)
	idx := strandIndex(GFF3Entry{
		Chr: chr, Start: 4001, End: 4500, Strand: "-", Type: "mRNA", Gene: "g1",
	})

	got, err := LookupWithIndex(idx, "db", "tx", dir, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// A minus-strand gene needs the upstream flank on the high-coordinate
	// side, then the whole window is reverse-complemented.
	if got.ScanStart != 4001 || got.ScanEnd != 9500 {
		t.Fatalf("scan window = %d-%d, want 4001-9500", got.ScanStart, got.ScanEnd)
	}
	if want := reverseComplement(fixtureSeq(chr, 4001, 9500)); got.Sequence != want {
		t.Errorf("sequence mismatch: got len %d, want len %d", len(got.Sequence), len(want))
	}
}

func TestLookupWithIndex_MinusStrandWindowClampsAtChromosomeEnd(t *testing.T) {
	const chr = "Chr1"
	dir := writeStrandFixture(t, chr, 12000)
	idx := strandIndex(GFF3Entry{
		Chr: chr, Start: 7001, End: 7500, Strand: "-", Type: "mRNA", Gene: "g1",
	})

	got, err := LookupWithIndex(idx, "db", "tx", dir, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// The requested high-coordinate flank runs past the chromosome end. The
	// returned scan_end must describe the bases that actually exist.
	if got.ScanStart != 7001 || got.ScanEnd != 12000 {
		t.Fatalf("scan window = %d-%d, want 7001-12000", got.ScanStart, got.ScanEnd)
	}
	if want := reverseComplement(fixtureSeq(chr, 7001, 12000)); got.Sequence != want {
		t.Errorf("sequence mismatch: got len %d, want len %d", len(got.Sequence), len(want))
	}
}
