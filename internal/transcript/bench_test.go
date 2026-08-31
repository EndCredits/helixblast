package transcript_test

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/EndCredits/helixblast/internal/prepare"
	"github.com/EndCredits/helixblast/internal/transcript"
)

// Baseline benchmarks for the per-request index-loading path
// (api.go: LoadIndexAuto → lookup → Close, once per request).
//
// Scale mirrors production claims: ~200K entries across 10 chromosomes.

const (
	benchGenes    = 66_000 // ×3 entries (gene/mRNA/CDS) ≈ 198K
	benchChroms   = 10
	benchChromLen = 50_000_000
)

func synthIndex() *transcript.GFF3Data {
	gff := &transcript.GFF3Data{
		Entries:    make(transcript.GFF3Index, benchGenes*3),
		Families:   make(transcript.GFF3Families, benchGenes),
		Coords:     make(transcript.GFF3Coords, benchGenes),
		FastaIndex: make(map[string]int64, benchChroms),
		Spatial:    make(transcript.GFF3Spatial, benchChroms),
	}
	for c := 0; c < benchChroms; c++ {
		chr := fmt.Sprintf("Chr%02d", c+1)
		gff.FastaIndex[chr] = int64(c) * int64(benchChromLen)
	}

	for i := 0; i < benchGenes; i++ {
		chr := fmt.Sprintf("Chr%02d", i%benchChroms+1)
		start := 1000 + (i/benchChroms)*3000
		end := start + 2000
		gene := fmt.Sprintf("g%06d", i)
		tx := gene + ".t1"
		cds := tx + ".CDS1"

		gff.Entries[gene] = transcript.GFF3Entry{Chr: chr, Start: start, End: end, Strand: "+", Type: "gene", Gene: gene}
		gff.Entries[tx] = transcript.GFF3Entry{Chr: chr, Start: start, End: end, Strand: "+", Type: "mRNA", Gene: gene}
		gff.Entries[cds] = transcript.GFF3Entry{Chr: chr, Start: start, End: end, Strand: "+", Type: "CDS", Gene: gene}
		gff.Families[gene] = transcript.GFF3Family{Transcripts: []string{tx}, CDSs: []string{cds}, Exons: []string{tx + ".exon1"}}
		gff.Coords[tx] = transcript.TranscriptRegions{
			Exons: []transcript.RegionCoord{{Start: start, End: start + 800}, {Start: start + 1200, End: end}},
			CDSs:  []transcript.RegionCoord{{Start: start + 100, End: end - 100}},
		}
		gff.Spatial[chr] = append(gff.Spatial[chr],
			transcript.SpatialFeature{Start: start, End: end, ID: gene, Type: "gene"},
			transcript.SpatialFeature{Start: start, End: end, ID: tx, Type: "mRNA"},
			transcript.SpatialFeature{Start: start + 100, End: end - 100, ID: cds, Type: "CDS"},
		)
	}
	for chr := range gff.Spatial {
		feats := gff.Spatial[chr]
		sort.Slice(feats, func(a, b int) bool { return feats[a].Start < feats[b].Start })
	}
	return gff
}

// buildIndexes writes bench.index.json.gz and bench.index.bin into a temp dir.
// The .bin is derived from the same data, so LoadIndexAuto(jsonPath) prefers it.
func buildIndexes(b *testing.B) (jsonPath, binPath string) {
	b.Helper()
	dir := b.TempDir()
	gff := synthIndex()

	jsonPath = filepath.Join(dir, "bench.index.json.gz")
	f, err := os.Create(jsonPath)
	if err != nil {
		b.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	if err := json.NewEncoder(gz).Encode(gff); err != nil {
		b.Fatal(err)
	}
	gz.Close()
	f.Close()

	binPath = filepath.Join(dir, "bench.index.bin")
	if err := prepare.BuildBinaryIndexFromData(gff, binPath); err != nil {
		b.Fatal(err)
	}
	return jsonPath, binPath
}

func fileSize(b *testing.B, path string) int64 {
	b.Helper()
	st, err := os.Stat(path)
	if err != nil {
		b.Fatal(err)
	}
	return st.Size()
}

// --- per-request load cost (what every /transcripts and /spatial call pays) ---

// BenchmarkRequestBinLoadOnly: mmap open + verify + munmap per request.
func BenchmarkRequestBinLoadOnly(b *testing.B) {
	_, binPath := buildIndexes(b)
	b.ReportMetric(float64(fileSize(b, binPath))/1e6, "MB/file")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := transcript.LoadIndexAuto(binPath)
		if err != nil {
			b.Fatal(err)
		}
		r.Close()
	}
}

// BenchmarkRequestJSONLoadOnly: full gzip+json decode per request (no .bin sibling).
func BenchmarkRequestJSONLoadOnly(b *testing.B) {
	jsonPath, binPath := buildIndexes(b)
	if err := os.Remove(binPath); err != nil { // force JSON fallback
		b.Fatal(err)
	}
	b.ReportMetric(float64(fileSize(b, jsonPath))/1e6, "MB/file")
	w, err := transcript.LoadIndexAuto(jsonPath) // warm the JSON cache
	if err != nil {
		b.Fatal(err)
	}
	w.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := transcript.LoadIndexAuto(jsonPath)
		if err != nil {
			b.Fatal(err)
		}
		r.Close()
	}
}

// --- steady-state lookup once a reader is resident (cache-hit simulation) ---

func BenchmarkLookupEntryResident(b *testing.B) {
	for _, kind := range []string{"bin", "json"} {
		b.Run(kind, func(b *testing.B) {
			jsonPath, binPath := buildIndexes(b)
			path := binPath
			if kind == "json" {
				path = jsonPath
				os.Remove(binPath)
			}
			r, err := transcript.LoadIndexAuto(path)
			if err != nil {
				b.Fatal(err)
			}
			defer r.Close()
			id := "g065999" // last gene, deterministic from synthIndex
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, ok := r.LookupEntry(id); !ok {
					b.Fatal("entry not found")
				}
			}
		})
	}
}

func BenchmarkSpatialV2Resident(b *testing.B) {
	for _, kind := range []string{"bin", "json"} {
		b.Run(kind, func(b *testing.B) {
			jsonPath, binPath := buildIndexes(b)
			path := binPath
			if kind == "json" {
				path = jsonPath
				os.Remove(binPath)
			}
			r, err := transcript.LoadIndexAuto(path)
			if err != nil {
				b.Fatal(err)
			}
			defer r.Close()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := transcript.SpatialLookupV2(r, "Chr03", 12_500_000); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// --- end-to-end request simulation: load + query + close (today's handler) ---

func BenchmarkRequestEndToEnd(b *testing.B) {
	for _, kind := range []string{"bin", "json"} {
		b.Run(kind, func(b *testing.B) {
			jsonPath, binPath := buildIndexes(b)
			path := binPath
			if kind == "json" {
				path = jsonPath
				os.Remove(binPath)
			}
			if w, err := transcript.LoadIndexAuto(path); err == nil { // warm
				w.Close()
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				r, err := transcript.LoadIndexAuto(path)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := transcript.SpatialLookupV2(r, "Chr03", 12_500_000); err != nil {
					b.Fatal(err)
				}
				r.Close()
			}
		})
	}
}
