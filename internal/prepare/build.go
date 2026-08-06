package prepare

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/EndCredits/helixblast/internal/index"
	"github.com/EndCredits/helixblast/internal/transcript"
)

type builder struct {
	f       *os.File
	strings strings.Builder
	poolCur uint64
	pool    map[string]uint32
}

func BuildBinaryIndex(jsonPath, binPath string) error {
	gff, err := transcript.LoadIndex(jsonPath)
	if err != nil {
		return fmt.Errorf("load json index: %w", err)
	}

	b := &builder{pool: make(map[string]uint32)}

	out, err := os.Create(binPath)
	if err != nil {
		return fmt.Errorf("create binary: %w", err)
	}
	defer out.Close()
	b.f = out

	reserve := int64(index.HeaderSize)
	if err := out.Truncate(reserve); err != nil {
		return fmt.Errorf("truncate: %w", err)
	}
	out.Seek(reserve, 0)

	entriesOff := uint64(reserve)
	entries := b.collectEntries(gff.Entries)
	entrySlots := b.buildEntryHash(entries)

	_, entrySlotCount := b.writeHashTable(entrySlots)
	entryDataOff := entriesOff + uint64(entrySlotCount)*index.HashSlotSize
	b.writeEntryRecords(entries)

	familiesOff := entryDataOff + uint64(len(entries))*index.EntryRecordSize
	families := b.collectFamilies(gff.Families)
	famSlots := b.buildFamilyHash(families)

	_, famSlotCount := b.writeHashTable(famSlots)
	famDataOff := familiesOff + uint64(famSlotCount)*index.HashSlotSize
	famOffsets := b.writeFamilyRecords(families)

	famStringsOff := famDataOff + uint64(len(families))*index.FamilyRecordSize
	famStringsStart := b.writeU32Rows(families, famOffsets, famStringsOff)

	coordsOff := famStringsStart
	coords := b.collectCoords(gff.Coords)
	coordSlots := b.buildCoordHash(coords)

	_, coordSlotCount := b.writeHashTable(coordSlots)
	coordDataOff := coordsOff + uint64(coordSlotCount)*index.HashSlotSize
	coordOffsets := b.writeCoordRecords(coords)

	coordPairsOff := coordDataOff + uint64(len(coords))*index.CoordRecordSize
	coordPairsEnd := b.writeCoordPairs(coords, coordOffsets, coordPairsOff)

	spatialOff := coordPairsEnd
	spChrs := b.collectSpatial(gff.Spatial)
	spHeadersEnd := spatialOff + uint64(4+len(spChrs)*index.SpatialHeaderSize)
	b.writeSpatial(spChrs, spHeadersEnd)

	spatialEnd := uint64(b.size())
	fastaOff := b.align(spatialEnd)
	fastaEnd := b.writeFastaIndex(gff.FastaIndex, fastaOff)

	poolOff := fastaEnd
	poolSize := b.writeStringPool()

	hdr := index.Header{
		EntryCount:     uint32(len(entries)),
		FamilyCount:    uint32(len(families)),
		CoordCount:     uint32(len(coords)),
		SpatialCount:   uint32(len(spChrs)),
		FastaChrCount:  uint32(len(gff.FastaIndex)),
		StringPoolSize: poolSize,
		EntriesOffset:  entriesOff,
		FamiliesOffset: familiesOff,
		CoordsOffset:   coordsOff,
		SpatialOffset:  spatialOff,
		FastaIdxOffset: fastaOff,
		StringPoolOff:  poolOff,
	}
	copy(hdr.Magic[:], index.Magic)
	hdr.Version = index.Version

	b.f.Seek(0, 0)
	binary.Write(b.f, index.ByteOrder(), &hdr)
	b.f.Sync()

	return nil
}

type indexedEntry struct {
	id  string
	rec index.EntryRecord
}

func (b *builder) collectEntries(m transcript.GFF3Index) []indexedEntry {
	result := make([]indexedEntry, 0, len(m))
	for id, e := range m {
		result = append(result, indexedEntry{
			id: id,
			rec: index.EntryRecord{
				ChrOffset:    b.stringRef(e.Chr),
				Start:        int32(e.Start),
				End:          int32(e.End),
				StrandOffset: b.stringRef(e.Strand),
				TypeOffset:   b.stringRef(e.Type),
				GeneOffset:   b.stringRef(e.Gene),
			},
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].id < result[j].id })
	return result
}

type indexedFamily struct {
	gene string
	rec  index.FamilyRecord
	tx   []string
	cds  []string
	exon []string
}

func (b *builder) collectFamilies(m transcript.GFF3Families) []indexedFamily {
	result := make([]indexedFamily, 0, len(m))
	for gene, f := range m {
		result = append(result, indexedFamily{
			gene: gene,
			rec:  index.FamilyRecord{TranscriptCount: uint32(len(f.Transcripts)), CDSCount: uint32(len(f.CDSs)), ExonCount: uint32(len(f.Exons))},
			tx:   f.Transcripts,
			cds:  f.CDSs,
			exon: f.Exons,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].gene < result[j].gene })
	return result
}

type indexedCoord struct {
	id    string
	rec   index.CoordRecord
	pairs []index.RegionPair
}

func (b *builder) collectCoords(m transcript.GFF3Coords) []indexedCoord {
	result := make([]indexedCoord, 0, len(m))
	for id, c := range m {
		pairs := make([]index.RegionPair, 0, len(c.Exons)+len(c.CDSs))
		for _, e := range c.Exons {
			pairs = append(pairs, index.RegionPair{Start: int32(e.Start), End: int32(e.End)})
		}
		for _, d := range c.CDSs {
			pairs = append(pairs, index.RegionPair{Start: int32(d.Start), End: int32(d.End)})
		}
		result = append(result, indexedCoord{
			id:    id,
			rec:   index.CoordRecord{ExonCount: uint32(len(c.Exons)), CDSCount: uint32(len(c.CDSs))},
			pairs: pairs,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].id < result[j].id })
	return result
}

type indexedSpatialChr struct {
	chr      string
	features []index.SpatialFeatureRec
}

func (b *builder) collectSpatial(m transcript.GFF3Spatial) []indexedSpatialChr {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]indexedSpatialChr, len(keys))
	for i, k := range keys {
		feats := m[k]
		recs := make([]index.SpatialFeatureRec, len(feats))
		for j, f := range feats {
			recs[j] = index.SpatialFeatureRec{Start: int32(f.Start), End: int32(f.End), IDOffset: b.stringRef(f.ID), TypeOffset: b.stringRef(f.Type)}
		}
		result[i] = indexedSpatialChr{chr: k, features: recs}
	}
	return result
}

func (b *builder) buildEntryHash(entries []indexedEntry) []index.HashSlot {
	cap := index.NextPow2(uint32(len(entries)) * 2)
	slots := make([]index.HashSlot, cap)
	for i, e := range entries {
		h := index.HashStr(e.id)
		pos := h % uint64(cap)
		for slots[pos].Hash != 0 {
			pos = (pos + 1) % uint64(cap)
		}
		slots[pos].Hash = h
		slots[pos].Val = (uint64(i) << 32) | uint64(b.stringRef(e.id))
	}
	return slots
}

func (b *builder) buildFamilyHash(families []indexedFamily) []index.HashSlot {
	cap := index.NextPow2(uint32(len(families)) * 2)
	slots := make([]index.HashSlot, cap)
	for i, f := range families {
		h := index.HashStr(f.gene)
		pos := h % uint64(cap)
		for slots[pos].Hash != 0 {
			pos = (pos + 1) % uint64(cap)
		}
		slots[pos].Hash = h
		slots[pos].Val = (uint64(i) << 32) | uint64(b.stringRef(f.gene))
	}
	return slots
}

func (b *builder) buildCoordHash(coords []indexedCoord) []index.HashSlot {
	cap := index.NextPow2(uint32(len(coords)) * 2)
	slots := make([]index.HashSlot, cap)
	for i, c := range coords {
		h := index.HashStr(c.id)
		pos := h % uint64(cap)
		for slots[pos].Hash != 0 {
			pos = (pos + 1) % uint64(cap)
		}
		slots[pos].Hash = h
		slots[pos].Val = (uint64(i) << 32) | uint64(b.stringRef(c.id))
	}
	return slots
}

func (b *builder) writeHashTable(slots []index.HashSlot) ([]index.HashSlot, uint32) {
	for _, s := range slots {
		binary.Write(b.f, index.ByteOrder(), &s)
	}
	return slots, uint32(len(slots))
}

func (b *builder) writeEntryRecords(entries []indexedEntry) [][8]uint32 {
	for _, e := range entries {
		binary.Write(b.f, index.ByteOrder(), &e.rec)
	}
	return nil
}

func (b *builder) writeFamilyRecords(families []indexedFamily) []uint64 {
	offsets := make([]uint64, len(families))
	for i, f := range families {
		off, _ := b.f.Seek(0, 1)
		offsets[i] = uint64(off)
		binary.Write(b.f, index.ByteOrder(), &f.rec)
	}
	return offsets
}

func (b *builder) writeU32Rows(families []indexedFamily, famOffsets []uint64, stringsStart uint64) uint64 {
	cur := stringsStart
	for i, f := range families {
		f.rec.DataOffset = cur
		buf := make([]uint32, 0, f.rec.TranscriptCount+f.rec.CDSCount+f.rec.ExonCount)
		for _, t := range f.tx {
			buf = append(buf, b.stringRef(t))
		}
		for _, c := range f.cds {
			buf = append(buf, b.stringRef(c))
		}
		for _, e := range f.exon {
			buf = append(buf, b.stringRef(e))
		}
		for _, v := range buf {
			binary.Write(b.f, index.ByteOrder(), v)
		}
		cur += uint64(len(buf) * 4)

		off, _ := b.f.Seek(0, 1)
		b.f.Seek(int64(famOffsets[i]), 0)
		binary.Write(b.f, index.ByteOrder(), f.rec)
		b.f.Seek(off, 0)
	}
	return cur
}

func (b *builder) writeCoordRecords(coords []indexedCoord) []uint64 {
	offsets := make([]uint64, len(coords))
	for i, c := range coords {
		off, _ := b.f.Seek(0, 1)
		offsets[i] = uint64(off)
		binary.Write(b.f, index.ByteOrder(), &c.rec)
	}
	return offsets
}

func (b *builder) writeCoordPairs(coords []indexedCoord, coordOffsets []uint64, pairsStart uint64) uint64 {
	cur := pairsStart
	for i, c := range coords {
		c.rec.DataOffset = cur
		for _, p := range c.pairs {
			binary.Write(b.f, index.ByteOrder(), &p)
		}
		cur += uint64(len(c.pairs) * index.RegionPairSize)

		off, _ := b.f.Seek(0, 1)
		b.f.Seek(int64(coordOffsets[i]), 0)
		binary.Write(b.f, index.ByteOrder(), c.rec)
		b.f.Seek(off, 0)
	}
	return cur
}

func (b *builder) writeSpatial(chrs []indexedSpatialChr, dataStart uint64) uint64 {
	binary.Write(b.f, index.ByteOrder(), uint32(len(chrs)))
	cur := dataStart
	headers := make([]index.SpatialHeader, len(chrs))
	for i, c := range chrs {
		headers[i] = index.SpatialHeader{ChrOffset: b.stringRef(c.chr), FeatureCount: uint32(len(c.features)), DataOffset: cur}
		cur += uint64(len(c.features) * index.SpatialFeatureRecSize)
	}
	for _, h := range headers {
		binary.Write(b.f, index.ByteOrder(), &h)
	}
	for _, c := range chrs {
		for _, f := range c.features {
			binary.Write(b.f, index.ByteOrder(), &f)
		}
	}
	return uint64(b.size())
}

func (b *builder) writeFastaIndex(m map[string]int64, start uint64) uint64 {
	binary.Write(b.f, index.ByteOrder(), uint32(len(m)))
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		e := index.FastaIndexEntry{ChrOffset: b.stringRef(k), Offset: m[k]}
		binary.Write(b.f, index.ByteOrder(), &e)
	}
	return uint64(b.size())
}

func (b *builder) writeStringPool() uint64 {
	b.f.Write([]byte(b.strings.String()))
	return uint64(b.strings.Len())
}

func (b *builder) size() int {
	b.f.Sync()
	off, _ := b.f.Seek(0, 1)
	return int(off)
}

func (b *builder) stringRef(s string) uint32 {
	if off, ok := b.pool[s]; ok {
		return off
	}
	off := b.poolCur
	b.pool[s] = uint32(off)
	b.strings.WriteString(s)
	b.strings.WriteByte(0)
	b.poolCur += uint64(len(s)) + 1
	return uint32(off)
}

func (b *builder) align(end uint64) uint64 {
	return end
}
