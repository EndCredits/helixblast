package transcript

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/EndCredits/helixblast/internal/index"
)

type IndexReader interface {
	LookupEntry(id string) (*index.Entry, bool)
	LookupFamily(gene string) (*index.Family, bool)
	LookupCoords(id string) (*index.CoordRegions, bool)
	SpatialSearch(chr string, start, end int) (*index.SpatialHits, error)
	FastaOffset(chr string) (int64, bool)
	FastaIndexMap() map[string]int64
	Close() error
}

func LoadIndexAuto(path string) (IndexReader, error) {
	if strings.HasSuffix(path, ".bin") {
		return index.Open(path)
	}

	binPath := strings.TrimSuffix(path, ".json.gz")
	binPath = strings.TrimSuffix(binPath, ".json")
	binPath += ".bin"

	if _, err := os.Stat(binPath); err == nil {
		return index.Open(binPath)
	}

	// JSON fallback: a full gzip+json decode per request is expensive
	// (hundreds of ms and heavy GC churn). Cache the decoded reader, keyed by
	// path and invalidated on mtime/size change. This is safe because
	// jsonIndexReader.Close() is a no-op — callers may Close a shared reader
	// without affecting others. Binary/mmap readers are deliberately NOT
	// cached: opening one is ~25µs and each owns an mmap that must be closed.
	return loadJSONCached(path)
}

type jsonCacheEntry struct {
	mu      sync.Mutex
	reader  *jsonIndexReader
	modTime time.Time
	size    int64
	valid   bool
}

var (
	jsonCacheMu sync.Mutex
	jsonCache   = map[string]*jsonCacheEntry{}
)

func loadJSONCached(path string) (IndexReader, error) {
	jsonCacheMu.Lock()
	entry, ok := jsonCache[path]
	if !ok {
		entry = &jsonCacheEntry{}
		jsonCache[path] = entry
	}
	jsonCacheMu.Unlock()

	entry.mu.Lock()
	defer entry.mu.Unlock()

	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat index file: %w", err)
	}
	if entry.valid && entry.modTime.Equal(st.ModTime()) && entry.size == st.Size() {
		return entry.reader, nil
	}

	gff, err := LoadIndex(path)
	if err != nil {
		return nil, err
	}
	entry.reader = &jsonIndexReader{data: gff}
	entry.modTime = st.ModTime()
	entry.size = st.Size()
	entry.valid = true
	return entry.reader, nil
}

// invalidateJSONCache clears all cached JSON readers. Used by tests and
// available for explicit cache resets.
func invalidateJSONCache() {
	jsonCacheMu.Lock()
	jsonCache = map[string]*jsonCacheEntry{}
	jsonCacheMu.Unlock()
}

type jsonIndexReader struct {
	data *GFF3Data
}

func (r *jsonIndexReader) LookupEntry(id string) (*index.Entry, bool) {
	e, ok := r.data.Entries[id]
	if !ok {
		return nil, false
	}
	return &index.Entry{Chr: e.Chr, Start: e.Start, End: e.End, Strand: e.Strand, Type: e.Type, Gene: e.Gene}, true
}

func (r *jsonIndexReader) LookupFamily(gene string) (*index.Family, bool) {
	f, ok := r.data.Families[gene]
	if !ok {
		return nil, false
	}
	return &index.Family{Transcripts: f.Transcripts, CDSs: f.CDSs, Exons: f.Exons}, true
}

func (r *jsonIndexReader) LookupCoords(id string) (*index.CoordRegions, bool) {
	c, ok := r.data.Coords[id]
	if !ok {
		return nil, false
	}
	out := &index.CoordRegions{}
	for _, e := range c.Exons {
		out.Exons = append(out.Exons, index.Region{Start: e.Start, End: e.End})
	}
	for _, d := range c.CDSs {
		out.CDSs = append(out.CDSs, index.Region{Start: d.Start, End: d.End})
	}
	return out, true
}

func (r *jsonIndexReader) SpatialSearch(chr string, start, end int) (*index.SpatialHits, error) {
	feats, ok := r.data.Spatial[chr]
	if !ok {
		return nil, fmt.Errorf("chromosome %s not found in spatial index", chr)
	}
	if start > end {
		start, end = end, start
	}
	n := len(feats)

	// upperBound on Start (builder guarantees start-sorted order)
	lo, hi := 0, n
	for lo < hi {
		mid := (lo + hi) / 2
		if feats[mid].Start <= end {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	ub := lo

	// single bounded backward pass: overlaps + upstream flank (see
	// index.Reader.SpatialSearch for the windowing assumption)
	out := &index.SpatialHits{Overlapping: make([]index.SpatialFeat, 0, 8)}
	bestEnd, bestIdx := -1, -1
	limit := start - spatialBackScan
	for i := ub - 1; i >= 0; i-- {
		if feats[i].Start < limit {
			break
		}
		switch {
		case feats[i].End >= start:
			out.Overlapping = append(out.Overlapping, index.SpatialFeat{
				Start: feats[i].Start, End: feats[i].End, ID: feats[i].ID, Type: feats[i].Type,
			})
		case feats[i].End >= bestEnd:
			bestEnd, bestIdx = feats[i].End, i
		}
	}
	for i, j := 0, len(out.Overlapping)-1; i < j; i, j = i+1, j-1 {
		out.Overlapping[i], out.Overlapping[j] = out.Overlapping[j], out.Overlapping[i]
	}
	if bestIdx >= 0 {
		f := index.SpatialFeat{
			Start: feats[bestIdx].Start, End: feats[bestIdx].End,
			ID: feats[bestIdx].ID, Type: feats[bestIdx].Type,
		}
		out.Upstream = &f
	}
	if ub < n {
		f := index.SpatialFeat{
			Start: feats[ub].Start, End: feats[ub].End,
			ID: feats[ub].ID, Type: feats[ub].Type,
		}
		out.Downstream = &f
	}
	return out, nil
}

// spatialBackScan must stay in sync with internal/index.Reader's window:
// bounds the leftward upstream search by coordinate distance.
const spatialBackScan = 8_000_000

func (r *jsonIndexReader) FastaOffset(chr string) (int64, bool) {
	off, ok := r.data.FastaIndex[chr]
	return off, ok
}

func (r *jsonIndexReader) FastaIndexMap() map[string]int64 {
	m := make(map[string]int64, len(r.data.FastaIndex))
	for k, v := range r.data.FastaIndex {
		m[k] = v
	}
	return m
}

func (r *jsonIndexReader) Close() error {
	return nil
}
