package transcript_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/EndCredits/helixblast/internal/index"
	"github.com/EndCredits/helixblast/internal/prepare"
	"github.com/EndCredits/helixblast/internal/transcript"
)

// naiveReader implements IndexReader with dead-simple O(n) scans. Used to
// cross-check the production binary-search implementations (unsafe mmap code
// and the JSON reader) against an obviously-correct reference.
type naiveReader struct {
	spatial map[string][]index.SpatialFeat
}

func (f *naiveReader) LookupEntry(string) (*index.Entry, bool)         { return nil, false }
func (f *naiveReader) LookupFamily(string) (*index.Family, bool)       { return nil, false }
func (f *naiveReader) LookupCoords(string) (*index.CoordRegions, bool) { return nil, false }
func (f *naiveReader) FastaOffset(string) (int64, bool)                { return 0, false }
func (f *naiveReader) FastaIndexMap() map[string]int64                 { return nil }
func (f *naiveReader) Close() error                                    { return nil }

func (f *naiveReader) SpatialSearch(chr string, start, end int) (*index.SpatialHits, error) {
	feats, ok := f.spatial[chr]
	if !ok {
		return nil, fmt.Errorf("chromosome %s not found in spatial index", chr)
	}
	if start > end {
		start, end = end, start
	}
	out := &index.SpatialHits{Overlapping: []index.SpatialFeat{}}
	bestEnd, upstream := -1, (*index.SpatialFeat)(nil)
	minStart, downstream := 1<<60, (*index.SpatialFeat)(nil)
	for i := range feats {
		f := feats[i]
		switch {
		case f.Start <= end && f.End >= start:
			cp := f
			out.Overlapping = append(out.Overlapping, cp)
		case f.End < start:
			if f.End > bestEnd {
				cp := f
				bestEnd, upstream = f.End, &cp
			}
		case f.Start > end:
			if f.Start < minStart {
				cp := f
				minStart, downstream = f.Start, &cp
			}
		}
	}
	out.Upstream, out.Downstream = upstream, downstream
	return out, nil
}

func feat(start, end int, id, typ string) index.SpatialFeat {
	return index.SpatialFeat{Start: start, End: end, ID: id, Type: typ}
}

// ---------------------------------------------------------------------------
// Semantics via the naive reference reader
// ---------------------------------------------------------------------------

func TestSpatial_PointQueryInclusiveBoundaries(t *testing.T) {
	r := &naiveReader{spatial: map[string][]index.SpatialFeat{
		"Chr1": {feat(100, 200, "g1", "gene")},
	}}
	for _, pos := range []int{100, 150, 200} {
		res, err := transcript.SpatialLookup(r, "Chr1", pos, pos)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Features) != 1 {
			t.Errorf("pos=%d: want 1 overlapping, got %d", pos, len(res.Features))
		}
	}
	res, _ := transcript.SpatialLookup(r, "Chr1", 201, 201)
	if len(res.Features) != 0 || res.Upstream == nil || res.Upstream.ID != "g1" {
		t.Errorf("pos=201: want upstream g1, got %+v", res)
	}
}

func TestSpatial_NestedUpstreamFixed(t *testing.T) {
	// Formerly BUG-PIN: upstream must be the feature with the GREATEST End
	// below the query, not the last one in start-sorted order.
	r := &naiveReader{spatial: map[string][]index.SpatialFeat{
		"Chr1": {
			feat(1000, 3000, "g1", "gene"),
			feat(1000, 3000, "g1.t1", "mRNA"),
			feat(1100, 2900, "g1.t1.CDS1", "CDS"),
			feat(5000, 6000, "g2", "gene"),
		},
	}}
	res, err := transcript.SpatialLookup(r, "Chr1", 3500, 3500)
	if err != nil {
		t.Fatal(err)
	}
	if res.Upstream == nil {
		t.Fatal("want upstream")
	}
	if res.Upstream.End != 3000 {
		t.Errorf("upstream End=%d, want 3000 (nearest by End, not innermost CDS)", res.Upstream.End)
	}
}

func TestSpatial_RangePartialOverlapReturnsFullLength(t *testing.T) {
	// A hit covering only the 3' end of a gene must return the gene's FULL
	// indexed span — never clipped to the query window.
	r := &naiveReader{spatial: map[string][]index.SpatialFeat{
		"Chr1": {feat(1000, 3000, "g1", "gene"), feat(4000, 5000, "g2", "gene")},
	}}
	res, err := transcript.SpatialLookup(r, "Chr1", 2500, 4500)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Features) != 2 {
		t.Fatalf("want both genes overlapping the range, got %d", len(res.Features))
	}
	if res.Features[0].Start != 1000 || res.Features[0].End != 3000 {
		t.Errorf("g1 must keep full span 1000-3000, got %d-%d", res.Features[0].Start, res.Features[0].End)
	}
}

func TestSpatial_RangeSpansIntergenicAndGenic(t *testing.T) {
	// The motivating case: a hit covering part gene + intergenic + part gene.
	// Both boundary genes must appear as overlapping (point queries used to
	// demote one to a flank or miss it entirely).
	r := &naiveReader{spatial: map[string][]index.SpatialFeat{
		"Chr1": {
			feat(1000, 2000, "a", "gene"),
			feat(5000, 6000, "b", "gene"),
			feat(9000, 10000, "c", "gene"),
		},
	}}
	res, _ := transcript.SpatialLookup(r, "Chr1", 1500, 9500)
	ids := map[string]bool{}
	for _, f := range res.Features {
		ids[f.ID] = true
	}
	if !ids["a"] || !ids["b"] || !ids["c"] {
		t.Errorf("range 1500-9500 must overlap a,b,c; got %v", ids)
	}
	if res.Upstream != nil || res.Downstream != nil {
		t.Errorf("flanks should be empty when range covers all: up=%v down=%v", res.Upstream, res.Downstream)
	}
}

func TestSpatial_ReversedRangeNormalized(t *testing.T) {
	// BLAST minus-strand HSPs arrive with subject_start > subject_end.
	r := &naiveReader{spatial: map[string][]index.SpatialFeat{
		"Chr1": {feat(1000, 3000, "g1", "gene")},
	}}
	res, err := transcript.SpatialLookup(r, "Chr1", 2500, 1500)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Features) != 1 {
		t.Errorf("reversed bounds must normalize, got %d features", len(res.Features))
	}
	if res.Start != 1500 || res.End != 2500 {
		t.Errorf("result should carry normalized bounds, got %d-%d", res.Start, res.End)
	}
}

func TestSpatial_FlanksAroundEmptyRange(t *testing.T) {
	r := &naiveReader{spatial: map[string][]index.SpatialFeat{
		"Chr1": {feat(100, 200, "a", "gene"), feat(300, 400, "b", "gene"), feat(700, 800, "c", "gene")},
	}}
	res, _ := transcript.SpatialLookup(r, "Chr1", 500, 600)
	if len(res.Features) != 0 || res.Upstream == nil || res.Upstream.ID != "b" ||
		res.Downstream == nil || res.Downstream.ID != "c" {
		t.Errorf("want upstream=b downstream=c, got %+v", res)
	}
	res, _ = transcript.SpatialLookup(r, "Chr1", 1, 50)
	if res.Upstream != nil || res.Downstream == nil || res.Downstream.ID != "a" {
		t.Errorf("pos=1-50: want only downstream=a, got %+v", res)
	}
	res, _ = transcript.SpatialLookup(r, "Chr1", 900, 1000)
	if res.Downstream != nil || res.Upstream == nil || res.Upstream.ID != "c" {
		t.Errorf("pos=900-1000: want only upstream=c, got %+v", res)
	}
}

func TestSpatial_EmptyChromosomeAndMissing(t *testing.T) {
	r := &naiveReader{spatial: map[string][]index.SpatialFeat{"ChrEmpty": {}}}
	res, err := transcript.SpatialLookup(r, "ChrEmpty", 100, 200)
	if err != nil {
		t.Fatalf("empty chromosome should not error: %v", err)
	}
	if res.Features == nil {
		t.Error("Features must be non-nil for JSON []")
	}
	if _, err := transcript.SpatialLookup(r, "Nope", 1, 2); err == nil {
		t.Error("missing chromosome should error")
	}
}

// ---------------------------------------------------------------------------
// Cross-check: production readers (unsafe mmap + JSON binary-search) against
// the naive reference on the synthetic 200K-entry genome.
// ---------------------------------------------------------------------------

func TestSpatialSearch_CrossCheckRealReaders(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a 200K-entry index")
	}
	gff := synthIndex()

	naive := &naiveReader{spatial: map[string][]index.SpatialFeat{}}
	for chr, feats := range gff.Spatial {
		fs := make([]index.SpatialFeat, len(feats))
		for i, f := range feats {
			fs[i] = feat(f.Start, f.End, f.ID, f.Type)
		}
		naive.spatial[chr] = fs
	}

	dir := t.TempDir()
	binPath := filepath.Join(dir, "x.index.bin")
	if err := prepare.BuildBinaryIndexFromData(gff, binPath); err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(dir, "x.index.json")
	b, err := json.Marshal(gff)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsonPath, b, 0o644); err != nil {
		t.Fatal(err)
	}

	binR, err := transcript.LoadIndexAuto(binPath)
	if err != nil {
		t.Fatal(err)
	}
	defer binR.Close()
	jsonR, err := transcript.LoadIndexAuto(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	defer jsonR.Close()

	rng := rand.New(rand.NewSource(42))
	chr := "Chr03"
	maxPos := 1000 + (benchGenes/benchChroms)*3000 + 5000
	queries := [][2]int{
		{1, 500}, {12_500_000, 12_500_000}, {12_499_999, 12_500_001},
		{maxPos + 1, maxPos + 100_000}, {1, maxPos},
	}
	for i := 0; i < 60; i++ {
		a := rng.Intn(maxPos) + 1
		queries = append(queries, [2]int{a, a + rng.Intn(50_000)})
	}

	for _, q := range queries {
		want, err := transcript.SpatialLookup(naive, chr, q[0], q[1])
		if err != nil {
			t.Fatal(err)
		}
		for name, r := range map[string]transcript.IndexReader{"bin": binR, "json": jsonR} {
			got, err := transcript.SpatialLookup(r, chr, q[0], q[1])
			if err != nil {
				t.Fatalf("%s %v: %v", name, q, err)
			}
			if !sameSpatialResult(got, want) {
				t.Errorf("%s query %v: production diverges from naive reference\ngot:  %+v\nwant: %+v", name, q, got, want)
			}
		}
	}
}

func sameSpatialResult(a, b *transcript.SpatialResult) bool {
	if len(a.Features) != len(b.Features) {
		return false
	}
	for i := range a.Features {
		if a.Features[i] != b.Features[i] {
			return false
		}
	}
	return sameFlank(a.Upstream, b.Upstream) && sameFlank(a.Downstream, b.Downstream)
}

func sameFlank(a, b *transcript.SpatialFeature) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
