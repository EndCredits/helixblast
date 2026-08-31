package transcript_test

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/EndCredits/helixblast/internal/prepare"
	"github.com/EndCredits/helixblast/internal/transcript"
)

func writeJSONGz(t *testing.T, path string, data *transcript.GFF3Data) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	if err := json.NewEncoder(gz).Encode(data); err != nil {
		t.Fatal(err)
	}
	gz.Close()
	f.Close()
}

func probeRSSKiB(t *testing.T) int64 {
	t.Helper()
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output()
	if err != nil {
		t.Skipf("ps unavailable: %v", err)
	}
	v, _ := strconv.ParseInt(string(out[:len(out)-1]), 10, 64)
	return v
}

func liveHeap() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

func totalAlloc() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.TotalAlloc
}

func TestMemoryProfile_JSONCache(t *testing.T) {
	if testing.Short() {
		t.Skip("memory profile builds a 200K-entry index")
	}
	dir := t.TempDir()

	// Build a ~50MB JSON index on disk, then drop all in-memory sources.
	gff := synthIndex()
	jsonPath := filepath.Join(dir, "probe.index.json.gz")
	writeJSONGz(t, jsonPath, gff)
	binPath := filepath.Join(dir, "probe.index.bin")
	if err := prepare.BuildBinaryIndexFromData(gff, binPath); err != nil {
		t.Fatal(err)
	}
	gff = nil

	jst, _ := os.Stat(jsonPath)
	bst, _ := os.Stat(binPath)
	t.Logf("on disk: json.gz=%.1fMB  bin=%.1fMB", float64(jst.Size())/1e6, float64(bst.Size())/1e6)

	baseHeap := liveHeap()
	baseRSS := probeRSSKiB(t)
	t.Logf("baseline: live-heap=%.1fMB  rss=%.0fMB", float64(baseHeap)/1e6, float64(baseRSS)/1024)

	// --- 1. cache resident cost (warm once) ---
	// Remove the sibling .bin: LoadIndexAuto prefers .bin over the JSON cache,
	// so its presence would make this probe measure mmap instead of the cache.
	if err := os.Remove(binPath); err != nil {
		t.Fatal(err)
	}
	r, err := transcript.LoadIndexAuto(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	warmHeap := liveHeap()
	t.Logf("[1] JSON cache warm: live-heap +%.1fMB  (resident cost of ONE cached index; RSS delta understated in-harness because the fixture already reserved arenas)",
		float64(warmHeap-baseHeap)/1e6)

	// --- 2. hit path churn: 1000 cached requests ---
	ta0 := totalAlloc()
	for i := 0; i < 1000; i++ {
		rr, err := transcript.LoadIndexAuto(jsonPath)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := rr.LookupEntry("g000007"); !ok {
			t.Fatal("entry missing")
		}
		rr.Close()
	}
	churn := float64(totalAlloc()-ta0) / 1000
	afterHits := liveHeap()
	t.Logf("[2] 1000 cache-hit requests: %.0f B/request cumulative alloc, live-heap drift %.1fMB",
		churn, float64(afterHits-warmHeap)/1e6)

	// --- 3. old behavior for contrast: uncached LoadIndex per request ---
	ta1 := totalAlloc()
	for i := 0; i < 3; i++ {
		rr, err := transcript.LoadIndex(jsonPath)
		if err != nil {
			t.Fatal(err)
		}
		_ = rr
	}
	oldChurn := float64(totalAlloc()-ta1) / 3
	t.Logf("[3] OLD per-request full decode: %.0f MB alloc/request (× latency 579ms)", oldChurn/1e6)

	// --- 4. bin path: request-time footprint and inter-request release ---
	if err := prepare.BuildBinaryIndexFromData(synthIndex(), binPath); err != nil {
		t.Fatal(err)
	}
	binBase := liveHeap()
	binRSS0 := probeRSSKiB(t)
	for i := 0; i < 1000; i++ {
		rr, err := transcript.LoadIndexAuto(binPath)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := transcript.SpatialLookupV2(rr, "Chr03", 12_500_000); err != nil {
			t.Fatal(err)
		}
		rr.Close()
	}
	binRSSDuring := probeRSSKiB(t)
	binHeapAfter := liveHeap()
	binRSSAfter := probeRSSKiB(t)
	t.Logf("[4] bin 1000 requests: rss during +%.0fMB, after +%.0fMB; live-heap drift %.1fMB",
		float64(binRSSDuring-binRSS0)/1024, float64(binRSSAfter-binRSS0)/1024,
		float64(binHeapAfter-binBase)/1e6)

	// --- 5. multi-db accumulation: 3 distinct cached indexes ---
	multiBase := liveHeap()
	for i := 0; i < 3; i++ {
		p := filepath.Join(dir, fmt.Sprintf("copy%d.json.gz", i))
		writeJSONGz(t, p, synthIndex())
		rr, err := transcript.LoadIndexAuto(p)
		if err != nil {
			t.Fatal(err)
		}
		rr.Close()
	}
	multiHeap := liveHeap()
	t.Logf("[5] 3 additional cached indexes: +%.1fMB live-heap (≈%.1fMB per index)",
		float64(multiHeap-multiBase)/1e6, float64(multiHeap-multiBase)/3e6)
}
