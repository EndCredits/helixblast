package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Fixture spec — mirrors internal/transcript/testdata/gen (re-implemented on
// purpose: the test doubles as an independent check of fixture content).
// ---------------------------------------------------------------------------

const fixtureAlphabet = "ACGT"

func fixtureSeed(c string) int {
	s := 0
	for i := 0; i < len(c); i++ {
		s += int(c[i])
	}
	return s % 4
}

func fixtureBase(c string, i int) byte {
	return fixtureAlphabet[(3*i*i+17*i+fixtureSeed(c))%4]
}

func fixtureSeq(c string, from, to int) string {
	var sb strings.Builder
	for i := from; i <= to; i++ {
		sb.WriteByte(fixtureBase(c, i))
	}
	return sb.String()
}

func fixturePath(name string) string { return filepath.Join("testdata", name) }

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// ---------------------------------------------------------------------------
// Core extraction semantics (unified streaming extractor)
// ---------------------------------------------------------------------------

func TestExtractRange_MultiRecord_LinearScan(t *testing.T) {
	got, err := extractRange(fixturePath("multi_wrapped.fa"), "Chr02", 1000, 1099, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := fixtureSeq("Chr02", 1000, 1099); got != want {
		t.Errorf("content mismatch")
	}
}

func TestExtractRange_MultiRecord_SeekHit(t *testing.T) {
	data, err := os.ReadFile(fixturePath("multi_wrapped.fa"))
	if err != nil {
		t.Fatal(err)
	}
	idx := map[string]int64{}
	for _, chr := range []string{"Chr01", "Chr02", "Chr03"} {
		if chr == "Chr01" {
			idx[chr] = 0
			continue
		}
		i := strings.Index(string(data), "\n>"+chr+" ")
		if i < 0 {
			t.Fatalf("marker for %s not found", chr)
		}
		idx[chr] = int64(i + 1)
	}
	got, err := extractRange(fixturePath("multi_wrapped.fa"), "Chr02", 5000, 5099, idx)
	if err != nil {
		t.Fatal(err)
	}
	if want := fixtureSeq("Chr02", 5000, 5099); got != want {
		t.Errorf("seek path content mismatch")
	}
}

func TestExtractRange_StaleSeek_FallsBack(t *testing.T) {
	// A bogus offset must NOT make the chromosome unreachable: the extractor
	// verifies the header and falls back to a linear scan.
	got, err := extractRange(fixturePath("multi_wrapped.fa"), "Chr02", 100, 199,
		map[string]int64{"Chr02": 12345})
	if err != nil {
		t.Fatalf("stale seek should fall back, got error: %v", err)
	}
	if want := fixtureSeq("Chr02", 100, 199); got != want {
		t.Errorf("fallback content mismatch")
	}
}

func TestExtractRange_PartialPastRecordEnd(t *testing.T) {
	// Chr02 is 6001 bases; requesting 5900-6010 returns the 102 available
	// bases and stops at Chr03's header.
	got, err := extractRange(fixturePath("multi_wrapped.fa"), "Chr02", 5900, 6010, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := fixtureSeq("Chr02", 5900, 6001); got != want || len(got) != 102 {
		t.Errorf("partial result: len=%d want 102", len(got))
	}
}

func TestExtractRange_MissingChromosome(t *testing.T) {
	_, err := extractRange(fixturePath("multi_wrapped.fa"), "Nope", 1, 10, nil)
	if err == nil || !strings.Contains(err.Error(), "chromosome Nope not found") {
		t.Errorf("want not-found error, got: %v", err)
	}
}

func TestExtractRange_FinalHeaderNoNewline(t *testing.T) {
	// The last header without a trailing newline must be recognized (the
	// chromosome exists) — requesting bases yields "range not found", not a
	// false "chromosome not found".
	path := writeTemp(t, "probe.fa", ">A\nACGTACGT\n>NoTrailer")
	_, err := extractRange(path, "NoTrailer", 1, 4, nil)
	if err == nil || !strings.Contains(err.Error(), "range 1-4 not found") {
		t.Errorf("want range-not-found (chr recognized), got: %v", err)
	}
}

func TestExtractRange_Boundaries(t *testing.T) {
	cases := []struct {
		name     string
		start    int
		end      int
		wantFrom int
		wantTo   int
	}{
		{"first bases", 1, 100, 1, 100},
		{"exact record end", 49901, 50000, 49901, 50000},
		{"single base", 25000, 25000, 25000, 25000},
		{"past end → partial", 49990, 50050, 49990, 50000},
	}
	for _, tc := range cases {
		for _, file := range []string{"single_wrapped.fa", "single_longline.fa"} {
			got, err := extractRange(fixturePath(file), "", tc.start, tc.end, nil)
			if err != nil {
				t.Errorf("%s/%s: %v", tc.name, file, err)
				continue
			}
			if want := fixtureSeq("Chr01", tc.wantFrom, tc.wantTo); got != want {
				t.Errorf("%s/%s: content mismatch (len %d vs %d)", tc.name, file, len(got), len(want))
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Format independence: the same genome must extract identically from every
// physical layout — the property the three legacy extractors could not
// guarantee (and failed outright beyond 10 MB single lines).
// ---------------------------------------------------------------------------

func TestFormatIndependence(t *testing.T) {
	ranges := [][2]int{{1, 100}, {12345, 12444}, {49901, 50000}, {49990, 50050}}
	for _, r := range ranges {
		want := fixtureSeq("Chr01", r[0], min(r[1], 50000))
		// multi_wrapped via targetChr (linear scan), and both single-record
		// files via "" (wrapped 70-col and one 50KB line)
		a, err := extractRange(fixturePath("multi_wrapped.fa"), "Chr01", r[0], r[1], nil)
		if err != nil {
			t.Fatalf("multi %v: %v", r, err)
		}
		b, err := extractRange(fixturePath("single_wrapped.fa"), "", r[0], r[1], nil)
		if err != nil {
			t.Fatalf("wrapped %v: %v", r, err)
		}
		c, err := extractRange(fixturePath("single_longline.fa"), "", r[0], r[1], nil)
		if err != nil {
			t.Fatalf("longline %v: %v", r, err)
		}
		if a != want || b != want || c != want {
			t.Errorf("range %v: format divergence (multi==want:%v wrapped==want:%v longline==want:%v)",
				r, a == want, b == want, c == want)
		}
	}
}

func TestExtractRange_ArbitraryLongLine(t *testing.T) {
	if testing.Short() {
		t.Skip("large fixture")
	}
	// 11 MB single line: previously failed with "token too long" (detection
	// blind spot + 10 MB scanner cap). Must now extract correctly.
	line := 11_000_000
	path := writeTemp(t, "big.fa", ">chr1\n"+strings.Repeat("A", line)+"\n")
	got, err := extractRange(path, "", 1_000_000, 1_000_100, nil)
	if err != nil {
		t.Fatalf("11MB single-line extraction failed: %v", err)
	}
	if len(got) != 101 || got != strings.Repeat("A", 101) {
		t.Errorf("bad result len=%d", len(got))
	}
}

func TestExtractRange_SecondHeaderEndsRecord(t *testing.T) {
	// '>' at any line start opens a new record; a second record's header can
	// never leak into the extracted bases (legacy chunked contamination bug).
	rec1 := strings.Repeat("ACGT", 50) // 200 bases
	rec2 := strings.Repeat("TGCA", 75)
	path := writeTemp(t, "two.fa", ">First\n"+rec1+"\n>Second record\n"+rec2+"\n")
	got, err := extractRange(path, "", 150, 260, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := rec1[149:]; got != want {
		t.Errorf("want %d clean trailing bases, got %q", len(want), got)
	}
}

func TestExtractRange_WhitespacePolicyUnified(t *testing.T) {
	// \r, space and tab are ignored everywhere, including inside lines —
	// previously Scanner/Multi kept internal spaces as bases while Chunked
	// stripped them (same input, different output per path).
	body := "AC GT\nACGTACGT\nACGTACGT\n"
	path := writeTemp(t, "space.fa", ">X\n"+body)
	got, err := extractRange(path, "", 1, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, " ") || got != "ACGTACGTAC" {
		t.Errorf("want space-free extraction, got %q", got)
	}

	// CRLF variant
	path = writeTemp(t, "crlf.fa", ">X\r\nACGT\r\nACGT\r\nACGT\r\n")
	got, err = extractRange(path, "", 2, 9, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "CGTACGTA" {
		t.Errorf("CRLF extraction: got %q", got)
	}
}

// ---------------------------------------------------------------------------
// extractSequence — dispatch + strand handling
// ---------------------------------------------------------------------------

func TestExtractSequence_PergromosomeFiles(t *testing.T) {
	dir := t.TempDir()
	src, err := os.ReadFile(fixturePath("single_wrapped.fa"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Chr01.fa"), src, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := extractSequence(dir, "", "Chr01", 100, 199, "+", nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := fixtureSeq("Chr01", 100, 199); got != want {
		t.Error("forward strand mismatch")
	}

	rev, err := extractSequence(dir, "", "Chr01", 100, 199, "-", nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := reverseComplement(fixtureSeq("Chr01", 100, 199)); rev != want {
		t.Error("reverse strand mismatch")
	}
}

func TestExtractSequence_NoSource(t *testing.T) {
	got, err := extractSequence("", "", "ChrX", 1, 10, "+", nil)
	if err != nil || got != "" {
		t.Errorf("want empty result without fasta config, got (%q, %v)", got, err)
	}
}

func TestReverseComplement_IUPAC(t *testing.T) {
	cases := map[string]string{
		"ATGC":   "GCAT",
		"atgc":   "gcat",
		"RYKM":   "KMRY",
		"ACGTN":  "NACGT",
		"ACGT-":  "-ACGT",
		"AAAAAC": "GTTTTT",
	}
	for in, want := range cases {
		if got := reverseComplement(in); got != want {
			t.Errorf("reverseComplement(%q) = %q, want %q", in, got, want)
		}
	}
}
