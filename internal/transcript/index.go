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
	Spatial(chr string) ([]index.SpatialFeat, error)
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

func (r *jsonIndexReader) Spatial(chr string) ([]index.SpatialFeat, error) {
	feats, ok := r.data.Spatial[chr]
	if !ok {
		return nil, fmt.Errorf("chromosome %s not found in spatial index", chr)
	}
	out := make([]index.SpatialFeat, len(feats))
	for i, f := range feats {
		out[i] = index.SpatialFeat{Start: f.Start, End: f.End, ID: f.ID, Type: f.Type}
	}
	return out, nil
}

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
