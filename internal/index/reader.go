package index

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

type Reader struct {
	data []byte
	f    *os.File
	hdr  Header
}

func Open(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open index: %w", err)
	}

	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat index: %w", err)
	}
	sz := fi.Size()
	if sz < HeaderSize {
		f.Close()
		return nil, fmt.Errorf("index too small: %d bytes", sz)
	}

	data, err := unix.Mmap(int(f.Fd()), 0, int(sz), unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("mmap index: %w", err)
	}

	r := &Reader{data: data, f: f}
	hdr := (*Header)(unsafe.Pointer(&data[0]))
	if string(hdr.Magic[:]) != Magic {
		r.Close()
		return nil, fmt.Errorf("bad magic: %q", string(hdr.Magic[:]))
	}
	if hdr.Version != Version {
		r.Close()
		return nil, fmt.Errorf("unsupported version: %d", hdr.Version)
	}
	r.hdr = *hdr
	return r, nil
}

func (r *Reader) Close() error {
	if r.data != nil {
		unix.Munmap(r.data)
		r.data = nil
	}
	if r.f != nil {
		r.f.Close()
		r.f = nil
	}
	return nil
}

func (r *Reader) stringAt(off uint32) string {
	pool := r.data[r.hdr.StringPoolOff : r.hdr.StringPoolOff+r.hdr.StringPoolSize]
	if int(off) >= len(pool) {
		return ""
	}
	end := off
	for end < uint32(len(pool)) && pool[end] != 0 {
		end++
	}
	return string(pool[off:end])
}

func (r *Reader) lookupHash(hashOff uint64, count uint32, id string) (int, bool) {
	h := hashStr(id)
	slots := unsafe.Slice((*HashSlot)(unsafe.Pointer(&r.data[hashOff])), count)
	pos := h % uint64(count)
	probed := uint32(0)
	for {
		slot := slots[pos]
		if slot.Hash == 0 {
			return 0, false
		}
		if slot.Hash == h {
			storedOff := uint32(slot.Val & 0xFFFFFFFF)
			if r.stringAt(storedOff) == id {
				return int(slot.Val >> 32), true
			}
		}
		pos = (pos + 1) % uint64(count)
		probed++
		if probed >= count {
			return 0, false
		}
	}
}

func (r *Reader) EntryCount() uint32 { return r.hdr.EntryCount }

func (r *Reader) LookupEntry(id string) (*Entry, bool) {
	idx, ok := r.lookupHash(r.hdr.EntriesOffset, r.entrySlotCount(), id)
	if !ok {
		return nil, false
	}
	if uint32(idx) >= r.hdr.EntryCount {
		return nil, false
	}
	recOff := r.hdr.EntriesOffset + uint64(r.entrySlotCount())*HashSlotSize + uint64(idx)*EntryRecordSize
	if int(recOff)+EntryRecordSize > len(r.data) {
		return nil, false
	}
	rec := (*EntryRecord)(unsafe.Pointer(&r.data[recOff]))
	return &Entry{
		Chr:    r.stringAt(rec.ChrOffset),
		Start:  int(rec.Start),
		End:    int(rec.End),
		Strand: r.stringAt(rec.StrandOffset),
		Type:   r.stringAt(rec.TypeOffset),
		Gene:   r.stringAt(rec.GeneOffset),
	}, true
}

type Entry struct {
	Chr    string
	Start  int
	End    int
	Strand string
	Type   string
	Gene   string
}

func (r *Reader) LookupFamily(gene string) (*Family, bool) {
	idx, ok := r.lookupHash(r.hdr.FamiliesOffset, r.familySlotCount(), gene)
	if !ok {
		return nil, false
	}
	if uint32(idx) >= r.hdr.FamilyCount {
		return nil, false
	}
	recOff := r.hdr.FamiliesOffset + uint64(r.familySlotCount())*HashSlotSize + uint64(idx)*FamilyRecordSize
	if int(recOff)+FamilyRecordSize > len(r.data) {
		return nil, false
	}
	rec := (*FamilyRecord)(unsafe.Pointer(&r.data[recOff]))
	if rec.DataOffset > uint64(len(r.data)) || int(rec.DataOffset)+int(rec.TranscriptCount+rec.CDSCount+rec.ExonCount)*4 > len(r.data) {
		return nil, false
	}
	f := &Family{
		Transcripts: make([]string, rec.TranscriptCount),
		CDSs:        make([]string, rec.CDSCount),
		Exons:       make([]string, rec.ExonCount),
	}
	off := rec.DataOffset
	n := rec.TranscriptCount
	for i := uint32(0); i < n; i++ {
		f.Transcripts[i] = r.stringAt(r.u32At(off))
		off += 4
	}
	n = rec.CDSCount
	for i := uint32(0); i < n; i++ {
		f.CDSs[i] = r.stringAt(r.u32At(off))
		off += 4
	}
	n = rec.ExonCount
	for i := uint32(0); i < n; i++ {
		f.Exons[i] = r.stringAt(r.u32At(off))
		off += 4
	}
	return f, true
}

type Family struct {
	Transcripts []string
	CDSs        []string
	Exons       []string
}

func (r *Reader) LookupCoords(id string) (*CoordRegions, bool) {
	idx, ok := r.lookupHash(r.hdr.CoordsOffset, r.coordSlotCount(), id)
	if !ok {
		return nil, false
	}
	if uint32(idx) >= r.hdr.CoordCount {
		return nil, false
	}
	recOff := r.hdr.CoordsOffset + uint64(r.coordSlotCount())*HashSlotSize + uint64(idx)*CoordRecordSize
	if int(recOff)+CoordRecordSize > len(r.data) {
		return nil, false
	}
	rec := (*CoordRecord)(unsafe.Pointer(&r.data[recOff]))
	if rec.DataOffset > uint64(len(r.data)) || int(rec.DataOffset)+int(rec.ExonCount+rec.CDSCount)*RegionPairSize > len(r.data) {
		return nil, false
	}
	c := &CoordRegions{}
	off := rec.DataOffset
	for i := uint32(0); i < rec.ExonCount; i++ {
		p := (*RegionPair)(unsafe.Pointer(&r.data[off]))
		c.Exons = append(c.Exons, Region{Start: int(p.Start), End: int(p.End)})
		off += RegionPairSize
	}
	for i := uint32(0); i < rec.CDSCount; i++ {
		p := (*RegionPair)(unsafe.Pointer(&r.data[off]))
		c.CDSs = append(c.CDSs, Region{Start: int(p.Start), End: int(p.End)})
		off += RegionPairSize
	}
	return c, true
}

type CoordRegions struct {
	Exons []Region
	CDSs  []Region
}

type Region struct {
	Start int
	End   int
}

func (r *Reader) Spatial(chr string) ([]SpatialFeat, error) {
	off := r.hdr.SpatialOffset
	n := r.u32At(off)
	off += 4
	for i := uint32(0); i < n; i++ {
		h := (*SpatialHeader)(unsafe.Pointer(&r.data[off]))
		if r.stringAt(h.ChrOffset) == chr {
			recs := unsafe.Slice((*SpatialFeatureRec)(unsafe.Pointer(&r.data[h.DataOffset])), h.FeatureCount)
			out := make([]SpatialFeat, h.FeatureCount)
			for j, rec := range recs {
				out[j] = SpatialFeat{Start: int(rec.Start), End: int(rec.End), ID: r.stringAt(rec.IDOffset), Type: r.stringAt(rec.TypeOffset)}
			}
			return out, nil
		}
		off += SpatialHeaderSize
	}
	return nil, fmt.Errorf("chromosome %s not found in spatial index", chr)
}

type SpatialFeat struct {
	Start int
	End   int
	ID    string
	Type  string
}

func (r *Reader) FastaOffset(chr string) (int64, bool) {
	off := r.hdr.FastaIdxOffset
	n := r.u32At(off)
	off += 4
	for i := uint32(0); i < n; i++ {
		e := (*FastaIndexEntry)(unsafe.Pointer(&r.data[off]))
		if r.stringAt(e.ChrOffset) == chr {
			return e.Offset, true
		}
		off += FastaIndexEntrySize
	}
	return 0, false
}

func (r *Reader) FastaIndexMap() map[string]int64 {
	off := r.hdr.FastaIdxOffset
	n := r.u32At(off)
	off += 4
	m := make(map[string]int64, n)
	for i := uint32(0); i < n; i++ {
		e := (*FastaIndexEntry)(unsafe.Pointer(&r.data[off]))
		m[r.stringAt(e.ChrOffset)] = e.Offset
		off += FastaIndexEntrySize
	}
	return m
}

func (r *Reader) u32At(off uint64) uint32 {
	return byteOrder.Uint32(r.data[off : off+4])
}

func (r *Reader) entrySlotCount() uint32  { return nextPow2(r.hdr.EntryCount * 2) }
func (r *Reader) familySlotCount() uint32 { return nextPow2(r.hdr.FamilyCount * 2) }
func (r *Reader) coordSlotCount() uint32  { return nextPow2(r.hdr.CoordCount * 2) }
