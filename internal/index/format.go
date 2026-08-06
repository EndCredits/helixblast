package index

import (
	"encoding/binary"
	"math/bits"

	"github.com/zeebo/xxh3"
)

const (
	Magic   = "HXBI"
	Version = 2 // v2: FNV-1a replaced with xxh3; v1 indices must be rebuilt
)

var byteOrder = binary.LittleEndian

func nextPow2(n uint32) uint32 {
	if n == 0 {
		return 1
	}
	return 1 << (32 - bits.LeadingZeros32(n-1))
}

type Header struct {
	Magic         [4]byte
	Version       uint32
	_             uint32
	EntryCount    uint32
	FamilyCount   uint32
	CoordCount    uint32
	SpatialCount  uint32
	FastaChrCount uint32
	StringPoolSize uint64
	EntriesOffset uint64
	FamiliesOffset uint64
	CoordsOffset  uint64
	SpatialOffset uint64
	FastaIdxOffset uint64
	StringPoolOff uint64
}

const HeaderSize = 88

type EntryRecord struct {
	ChrOffset    uint32
	Start        int32
	End          int32
	StrandOffset uint32
	TypeOffset   uint32
	GeneOffset   uint32
}

const EntryRecordSize = 24

type FamilyRecord struct {
	TranscriptCount uint32
	CDSCount        uint32
	ExonCount       uint32
	_               uint32 // align DataOffset to 8
	DataOffset      uint64
}

const FamilyRecordSize = 24

type CoordRecord struct {
	ExonCount  uint32
	CDSCount   uint32
	DataOffset uint64
}

const CoordRecordSize = 16

type RegionPair struct {
	Start int32
	End   int32
}

const RegionPairSize = 8

type SpatialHeader struct {
	ChrOffset    uint32
	FeatureCount uint32
	DataOffset   uint64
}

const SpatialHeaderSize = 16

type SpatialFeatureRec struct {
	Start      int32
	End        int32
	IDOffset   uint32
	TypeOffset uint32
}

const SpatialFeatureRecSize = 16

type FastaIndexEntry struct {
	ChrOffset uint32
	_         uint32 // align Offset to 8
	Offset    int64
}

const FastaIndexEntrySize = 16

type HashSlot struct {
	Hash uint64
	Val  uint64
}

const HashSlotSize = 16

func NextPow2(n uint32) uint32 {
	return nextPow2(n)
}

func ByteOrder() binary.ByteOrder {
	return byteOrder
}

// HashStr returns the 64-bit hash used for all hash tables in the index.
// It uses xxh3 with a zero-remap: hash value 0 is reserved as the empty-slot
// sentinel, so any xxh3 result of 0 is remapped to 1.
func HashStr(s string) uint64 {
	return hashNonZero(xxh3.HashString(s))
}

func hashNonZero(h uint64) uint64 {
	if h == 0 {
		return 1
	}
	return h
}
