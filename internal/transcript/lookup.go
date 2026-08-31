package transcript

import (
	"fmt"

	"github.com/EndCredits/helixblast/internal/index"
)

func LookupWithIndex(idx IndexReader, dbName string, transcriptID string, fastaDir string, fastaFile string, fastaIndex map[string]int64) (*Result, error) {
	entry, ok := idx.LookupEntry(transcriptID)
	if !ok {
		return nil, fmt.Errorf("ID %s not found in %s", transcriptID, dbName)
	}

	scanStart := entry.Start - 5000
	if scanStart < 1 {
		scanStart = 1
	}

	seq, err := extractSequence(fastaDir, fastaFile, entry.Chr, scanStart, entry.End, entry.Strand, fastaIndex)
	if err != nil {
		return nil, fmt.Errorf("extract sequence: %w", err)
	}

	result := &Result{
		TranscriptID: transcriptID,
		Database:     dbName,
		Chromosome:   entry.Chr,
		Start:        entry.Start,
		End:          entry.End,
		Strand:       entry.Strand,
		Type:         entry.Type,
		GeneID:       entry.Gene,
		Sequence:     seq,
		ScanStart:    scanStart,
		ScanEnd:      entry.End,
	}

	if regionCoords, ok := idx.LookupCoords(transcriptID); ok {
		result.Regions = &Regions{
			Exons: convertRegions(regionCoords.Exons),
			CDSs:  convertRegions(regionCoords.CDSs),
		}
	} else if family, ok := idx.LookupFamily(entry.Gene); ok {
		for _, t := range family.Transcripts {
			if rc, ok := idx.LookupCoords(t); ok {
				result.Regions = &Regions{
					Exons: convertRegions(rc.Exons),
					CDSs:  convertRegions(rc.CDSs),
				}
				break
			}
		}
	}

	if family, ok := idx.LookupFamily(entry.Gene); ok {
		result.Related = &RelatedIDs{
			Transcripts: family.Transcripts,
			CDSs:        family.CDSs,
			Exons:       family.Exons,
		}
	}

	return result, nil
}

func convertRegions(rs []index.Region) []RegionCoord {
	out := make([]RegionCoord, len(rs))
	for i, r := range rs {
		out[i] = RegionCoord{Start: r.Start, End: r.End}
	}
	return out
}

// SpatialLookup answers a genomic range query [start,end] (a point query
// passes start==end; reversed bounds are normalized — BLAST minus-strand
// HSPs have subject_start > subject_end). Every returned feature carries its
// full indexed span, never clipped to the query window: overlapping features
// intersect the range, upstream is the feature with the greatest End below
// the range, downstream the feature with the smallest Start above it.
func SpatialLookup(idx IndexReader, chr string, start, end int) (*SpatialResult, error) {
	if start > end {
		start, end = end, start
	}
	hits, err := idx.SpatialSearch(chr, start, end)
	if err != nil {
		return nil, err
	}

	features := make([]SpatialFeature, len(hits.Overlapping))
	for i, f := range hits.Overlapping {
		features[i] = SpatialFeature{Start: f.Start, End: f.End, ID: f.ID, Type: f.Type}
	}

	result := &SpatialResult{
		Chromosome: chr,
		Start:      start,
		End:        end,
		Features:   features,
	}
	if hits.Upstream != nil {
		result.Upstream = &SpatialFeature{
			Start: hits.Upstream.Start, End: hits.Upstream.End,
			ID: hits.Upstream.ID, Type: hits.Upstream.Type,
		}
	}
	if hits.Downstream != nil {
		result.Downstream = &SpatialFeature{
			Start: hits.Downstream.Start, End: hits.Downstream.End,
			ID: hits.Downstream.ID, Type: hits.Downstream.Type,
		}
	}
	return result, nil
}
