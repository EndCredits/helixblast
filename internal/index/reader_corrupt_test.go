package index_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/EndCredits/helixblast/internal/index"
	"github.com/EndCredits/helixblast/internal/prepare"
	"github.com/EndCredits/helixblast/internal/transcript"
)

func buildValidBin(t *testing.T) string {
	t.Helper()
	gff := &transcript.GFF3Data{
		Entries: transcript.GFF3Index{
			"g1":    {Chr: "Chr01", Start: 100, End: 500, Strand: "+", Type: "gene", Gene: "g1"},
			"g1.t1": {Chr: "Chr01", Start: 100, End: 500, Strand: "+", Type: "mRNA", Gene: "g1"},
		},
		Families: transcript.GFF3Families{
			"g1": {Transcripts: []string{"g1.t1"}, CDSs: []string{"g1.t1.CDS1"}, Exons: []string{"g1.t1.exon1"}},
		},
		Coords: transcript.GFF3Coords{
			"g1.t1": {Exons: []transcript.RegionCoord{{Start: 100, End: 300}, {Start: 400, End: 500}},
				CDSs: []transcript.RegionCoord{{Start: 150, End: 450}}},
		},
		FastaIndex: map[string]int64{"Chr01": 0},
		Spatial: transcript.GFF3Spatial{
			"Chr01": {
				{Start: 100, End: 500, ID: "g1", Type: "gene"},
				{Start: 100, End: 500, ID: "g1.t1", Type: "mRNA"},
			},
		},
	}
	path := filepath.Join(t.TempDir(), "valid.index.bin")
	if err := prepare.BuildBinaryIndexFromData(gff, path); err != nil {
		t.Fatal(err)
	}
	return path
}

func copyCorrupt(t *testing.T, src string, mutate func(data []byte) []byte) string {
	t.Helper()
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "corrupt.index.bin")
	if err := os.WriteFile(path, mutate(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpenRejectsTruncatedFile(t *testing.T) {
	valid := buildValidBin(t)
	fi, _ := os.Stat(valid)
	for _, frac := range []int64{10, 33, 66, 90} {
		cut := fi.Size() * frac / 100
		if cut < index.HeaderSize {
			continue
		}
		path := copyCorrupt(t, valid, func(b []byte) []byte { return b[:cut] })
		r, err := index.Open(path)
		if err == nil {
			r.Close()
			t.Errorf("truncation to %d%%: expected error", frac)
		}
	}
}

func TestOpenRejectsCorruptHeaderFields(t *testing.T) {
	valid := buildValidBin(t)
	// Field offsets (little-endian uint32/uint64) inside the 88-byte header.
	cases := []struct {
		name   string
		off    int
		size   int // 4 or 8
		mutate func(v uint64) uint64
	}{
		{"EntryCount huge", 12, 4, func(uint64) uint64 { return 0xFFFFFFFF }},
		{"FamilyCount huge", 16, 4, func(uint64) uint64 { return 0xFFFFFFFF }},
		{"CoordCount huge", 20, 4, func(uint64) uint64 { return 0xFFFFFFFF }},
		{"SpatialCount huge", 24, 4, func(uint64) uint64 { return 0xFFFFFFFF }},
		{"FastaChrCount huge", 28, 4, func(uint64) uint64 { return 0xFFFFFFFF }},
		{"StringPoolSize huge", 32, 8, func(uint64) uint64 { return 1 << 62 }},
		{"EntriesOffset huge", 40, 8, func(uint64) uint64 { return 1 << 62 }},
		{"FamiliesOffset huge", 48, 8, func(uint64) uint64 { return 1 << 62 }},
		{"CoordsOffset huge", 56, 8, func(uint64) uint64 { return 1 << 62 }},
		{"SpatialOffset huge", 64, 8, func(uint64) uint64 { return 1 << 62 }},
		{"FastaIdxOffset huge", 72, 8, func(uint64) uint64 { return 1 << 62 }},
		{"StringPoolOff huge", 80, 8, func(uint64) uint64 { return 1 << 62 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := copyCorrupt(t, valid, func(b []byte) []byte {
				v := tc.mutate(0)
				if tc.size == 4 {
					binary.LittleEndian.PutUint32(b[tc.off:], uint32(v))
				} else {
					binary.LittleEndian.PutUint64(b[tc.off:], v)
				}
				return b
			})
			r, err := index.Open(path)
			if err == nil {
				r.Close()
				t.Errorf("%s: expected Open to reject", tc.name)
			}
		})
	}
}

func TestOpenCorruptSpatialRecordOffsets(t *testing.T) {
	valid := buildValidBin(t)
	// Walk the spatial header table and blow up each DataOffset in turn.
	raw, _ := os.ReadFile(valid)
	fi, _ := os.Stat(valid)
	spatialOff := binary.LittleEndian.Uint64(raw[64:])
	n := binary.LittleEndian.Uint32(raw[spatialOff:])
	if n == 0 {
		t.Skip("fixture has no spatial chromosomes")
	}
	for i := uint32(0); i < n; i++ {
		dataOffField := spatialOff + 4 + uint64(i)*16 + 8
		path := copyCorrupt(t, valid, func(b []byte) []byte {
			binary.LittleEndian.PutUint64(b[dataOffField:], uint64(fi.Size())+1000)
			return b
		})
		r, err := index.Open(path)
		if err == nil {
			r.Close()
			t.Errorf("spatial chr %d: expected rejection of out-of-bounds DataOffset", i)
		}
	}
}

// HeaderByteFlip walks every byte of the header region, setting it to 0xFF,
// and asserts Open either rejects the file or behaves safely (no panic).
func TestOpenHeaderByteFlipNoPanic(t *testing.T) {
	valid := buildValidBin(t)
	for off := 4; off < 88; off++ { // skip magic (0-3): keep it valid
		path := copyCorrupt(t, valid, func(b []byte) []byte {
			b[off] = 0xFF
			return b
		})
		r, err := index.Open(path)
		if err == nil {
			// Accepted despite the flip (byte was padding/low-order): every
			// accessor must still be safe.
			r.LookupEntry("g1")
			r.LookupFamily("g1")
			r.LookupCoords("g1.t1")
			_, _ = r.SpatialSearch("Chr01", 150, 250)
			r.FastaOffset("Chr01")
			r.FastaIndexMap()
			r.Close()
		}
	}
}
