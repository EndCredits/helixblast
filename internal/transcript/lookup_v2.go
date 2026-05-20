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

func SpatialLookupV2(idx IndexReader, chr string, pos int) (*SpatialResult, error) {
	features, err := idx.Spatial(chr)
	if err != nil {
		return nil, err
	}

	overlapping := make([]SpatialFeature, 0)
	var upstream, downstream *SpatialFeature

	for i := range features {
		f := features[i]
		if f.Start <= pos && pos <= f.End {
			overlapping = append(overlapping, SpatialFeature{Start: f.Start, End: f.End, ID: f.ID, Type: f.Type})
		}
		if f.End < pos {
			c := SpatialFeature{Start: f.Start, End: f.End, ID: f.ID, Type: f.Type}
			upstream = &c
		}
		if f.Start > pos && downstream == nil {
			c := SpatialFeature{Start: f.Start, End: f.End, ID: f.ID, Type: f.Type}
			downstream = &c
			break
		}
	}

	return &SpatialResult{
		Chromosome: chr,
		Position:   pos,
		Features:   overlapping,
		Upstream:   upstream,
		Downstream: downstream,
	}, nil
}
