package transcript

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type GFF3Entry struct {
	Chr    string `json:"chr"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Strand string `json:"strand"`
	Type   string `json:"type"`
	Gene   string `json:"gene"`
}

type GFF3Index map[string]GFF3Entry

type GFF3Family struct {
	Transcripts []string `json:"transcripts"`
	CDSs        []string `json:"cdss"`
	Exons       []string `json:"exons"`
}

type GFF3Families map[string]GFF3Family

type GFF3Data struct {
	Entries    GFF3Index        `json:"entries"`
	Families   GFF3Families     `json:"families"`
	Coords     GFF3Coords       `json:"coords"`
	FastaIndex map[string]int64 `json:"fasta_index"`
	Spatial    GFF3Spatial      `json:"spatial"`
}

type GFF3Spatial map[string][]SpatialFeature

type SpatialFeature struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	ID    string `json:"id"`
	Type  string `json:"type"`
}

type SpatialResult struct {
	Chromosome string           `json:"chromosome"`
	Position   int              `json:"position"`
	Features   []SpatialFeature `json:"features"`
	Upstream   *SpatialFeature  `json:"upstream,omitempty"`
	Downstream *SpatialFeature  `json:"downstream,omitempty"`
}

func SpatialLookup(gffData *GFF3Data, chr string, pos int) (*SpatialResult, error) {
	features, ok := gffData.Spatial[chr]
	if !ok {
		return nil, fmt.Errorf("chromosome %s not found in spatial index", chr)
	}

	overlapping := make([]SpatialFeature, 0)
	var upstream, downstream *SpatialFeature

	for i := range features {
		f := features[i]
		if f.Start <= pos && pos <= f.End {
			overlapping = append(overlapping, f)
		}
		if f.End < pos {
			c := f
			upstream = &c
		}
		if f.Start > pos && downstream == nil {
			c := f
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

type GFF3Coords map[string]TranscriptRegions

type TranscriptRegions struct {
	Exons []RegionCoord `json:"exons"`
	CDSs  []RegionCoord `json:"cdss"`
}

type RegionCoord struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type RelatedIDs struct {
	Transcripts []string `json:"transcripts"`
	CDSs        []string `json:"cdss"`
	Exons       []string `json:"exons"`
}

type Result struct {
	TranscriptID string      `json:"transcript_id"`
	Database     string      `json:"database"`
	Chromosome   string      `json:"chromosome"`
	Start        int         `json:"start"`
	End          int         `json:"end"`
	Strand       string      `json:"strand"`
	Type         string      `json:"type"`
	GeneID       string      `json:"gene_id"`
	Sequence     string      `json:"sequence"`
	ScanStart    int         `json:"scan_start"`
	ScanEnd      int         `json:"scan_end"`
	Regions      *Regions    `json:"regions,omitempty"`
	Related      *RelatedIDs `json:"related,omitempty"`
}

type Regions struct {
	Exons []RegionCoord `json:"exons"`
	CDSs  []RegionCoord `json:"cdss"`
}

func LoadIndex(indexPath string) (*GFF3Data, error) {
	f, err := os.Open(indexPath)
	if err != nil {
		return nil, fmt.Errorf("open index file: %w", err)
	}
	defer f.Close()

	var reader io.Reader = f

	if strings.HasSuffix(indexPath, ".gz") {
		gzReader, err := gzip.NewReader(f)
		if err != nil {
			return nil, fmt.Errorf("create gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	var gffData GFF3Data
	if err := json.NewDecoder(reader).Decode(&gffData); err != nil {
		return nil, fmt.Errorf("parse index: %w", err)
	}

	return &gffData, nil
}

func Lookup(gffData *GFF3Data, dbName string, transcriptID string, fastaDir string, fastaFile string) (*Result, error) {
	entry, ok := gffData.Entries[transcriptID]
	if !ok {
		return nil, fmt.Errorf("ID %s not found in %s", transcriptID, dbName)
	}

	scanStart := entry.Start - 5000
	if scanStart < 1 {
		scanStart = 1
	}

	seq, err := extractSequence(fastaDir, fastaFile, entry.Chr, scanStart, entry.End, entry.Strand, gffData.FastaIndex)
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

	if regionCoords, ok := gffData.Coords[transcriptID]; ok {
		result.Regions = &Regions{
			Exons: regionCoords.Exons,
			CDSs:  regionCoords.CDSs,
		}
	} else if family, ok := gffData.Families[entry.Gene]; ok {
		for _, t := range family.Transcripts {
			if rc, ok := gffData.Coords[t]; ok {
				result.Regions = &Regions{
					Exons: rc.Exons,
					CDSs:  rc.CDSs,
				}
				break
			}
		}
	}

	if family, ok := gffData.Families[entry.Gene]; ok {
		result.Related = &RelatedIDs{
			Transcripts: family.Transcripts,
			CDSs:        family.CDSs,
			Exons:       family.Exons,
		}
	}

	return result, nil
}

func extractSequence(fastaDir, fastaFile, chr string, start, end int, strand string, fastaIndex map[string]int64) (string, error) {
	if start < 1 {
		start = 1
	}

	if fastaDir != "" {
		chrPath := filepath.Join(fastaDir, chr+".fa")
		if _, statErr := os.Stat(chrPath); statErr == nil {
			region, err := extractRange(chrPath, "", start, end, nil)
			if err != nil {
				return "", err
			}
			if strand == "-" {
				return reverseComplement(region), nil
			}
			return region, nil
		}
	}

	if fastaFile != "" {
		region, err := extractRange(fastaFile, chr, start, end, fastaIndex)
		if err != nil {
			return "", err
		}
		if strand == "-" {
			return reverseComplement(region), nil
		}
		return region, nil
	}

	return "", nil
}

func extractRange(path, targetChr string, start, end int, fastaIndex map[string]int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	if targetChr != "" {
		return extractRangeMulti(f, targetChr, start, end, fastaIndex)
	}

	if isLongLineFASTA(f) {
		f.Close()
		f, err = os.Open(path)
		if err != nil {
			return "", fmt.Errorf("reopen %s: %w", path, err)
		}
		defer f.Close()
		return extractRangeChunked(f, targetChr, start, end)
	}

	f.Close()
	f, err = os.Open(path)
	if err != nil {
		return "", fmt.Errorf("reopen %s: %w", path, err)
	}
	defer f.Close()
	return extractRangeScanner(f, targetChr, start, end)
}

func extractRangeMulti(f *os.File, targetChr string, start, end int, fastaIndex map[string]int64) (string, error) {
	reader := bufio.NewReaderSize(f, 1*1024*1024)

	if offset, ok := fastaIndex[targetChr]; ok {
		f.Seek(offset, 0)
		reader = bufio.NewReaderSize(f, 1*1024*1024)
		// Verify header
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("seek to %s failed: %w", targetChr, err)
		}
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, ">") {
			return "", fmt.Errorf("expected header at offset %d, got: %s", offset, trimmed)
		}
	} else {
		// Phase 1a: fast scan for target chromosome header
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return "", fmt.Errorf("chromosome %s not found in %s", targetChr, f.Name())
			}

			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, ">") {
				header := strings.TrimPrefix(trimmed, ">")
				header = strings.Split(header, " ")[0]
				if header == targetChr {
					break
				}
			}
		}
	}

	// Phase 2: extract range from target chromosome lines
	var sb strings.Builder
	need := end - start + 1
	skipped := 0

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ">") {
			break
		}

		lineLen := len(trimmed)

		if skipped+lineLen < start {
			skipped += lineLen
			continue
		}

		offset := start - skipped - 1
		if offset < 0 {
			offset = 0
		}

		remaining := need - sb.Len()
		chunk := trimmed
		endIdx := offset + remaining
		if endIdx < lineLen {
			chunk = trimmed[offset:endIdx]
		} else if offset > 0 {
			chunk = trimmed[offset:]
		}

		sb.WriteString(chunk)
		skipped += lineLen

		if sb.Len() >= need {
			return sb.String()[:need], nil
		}
	}

	if sb.Len() > 0 {
		return sb.String(), nil
	}

	return "", fmt.Errorf("range %d-%d not found in %s", start, end, f.Name())
}

func isLongLineFASTA(f *os.File) bool {
	scanner := bufio.NewScanner(f)
	n := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, ">") {
			continue
		}
		if len(line) > 120 {
			return true
		}
		n++
		if n >= 200 {
			break
		}
	}
	return false
}

func extractRangeScanner(f *os.File, targetChr string, start, end int) (string, error) {
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	var sb strings.Builder
	inTarget := targetChr == ""
	skipped := 0
	need := end - start + 1

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, ">") {
			if sb.Len() >= need {
				return sb.String()[:need], nil
			}
			if targetChr != "" {
				header := strings.TrimPrefix(line, ">")
				header = strings.Split(header, " ")[0]
				inTarget = header == targetChr
				skipped = 0
			}
			continue
		}

		if !inTarget {
			continue
		}

		lineLen := len(line)

		if skipped+lineLen < start {
			skipped += lineLen
			continue
		}

		offset := start - skipped - 1
		if offset < 0 {
			offset = 0
		}

		remaining := need - sb.Len()
		chunk := line
		endIdx := offset + remaining
		if endIdx < lineLen {
			chunk = line[offset:endIdx]
		} else if offset > 0 {
			chunk = line[offset:]
		}

		sb.WriteString(chunk)
		skipped += lineLen

		if sb.Len() >= need {
			return sb.String()[:need], nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read %s: %w", f.Name(), err)
	}

	if sb.Len() > 0 {
		return sb.String(), nil
	}

	return "", fmt.Errorf("range %d-%d not found in %s", start, end, f.Name())
}

func extractRangeChunked(f *os.File, targetChr string, start, end int) (string, error) {
	reader := bufio.NewReaderSize(f, 64*1024)

	var sb strings.Builder
	var headerBuf strings.Builder
	inTarget := targetChr == ""
	skipped := 0
	need := end - start + 1
	inHeader := false
	totalRead := 0

	buf := make([]byte, 64*1024)

	for {
		n, err := reader.Read(buf)
		if n == 0 && err != nil {
			break
		}

		for i := 0; i < n; i++ {
			ch := buf[i]

			if ch == '\n' {
				if inHeader {
					line := strings.TrimSpace(headerBuf.String())
					headerBuf.Reset()
					inHeader = false
					if strings.HasPrefix(line, ">") {
						if targetChr != "" {
							header := strings.TrimPrefix(line, ">")
							header = strings.Split(header, " ")[0]
							inTarget = header == targetChr
							skipped = 0
						}
						continue
					}
					// non-header line data already processed char by char below
				}
				continue
			}

			if ch == '>' && !inHeader && totalRead == 0 {
				inHeader = true
				headerBuf.WriteByte(ch)
				continue
			}

			if inHeader {
				headerBuf.WriteByte(ch)
				continue
			}

			if ch == '\r' || ch == ' ' || ch == '\t' {
				continue
			}

			totalRead++

			if !inTarget {
				if targetChr != "" && ch == '>' {
					// This should not happen within a sequence line
				}
				continue
			}

			if skipped+1 < start {
				skipped++
				continue
			}

			if sb.Len() >= need {
				return sb.String()[:need], nil
			}

			sb.WriteByte(ch)
			skipped++
		}

		if sb.Len() >= need {
			return sb.String()[:need], nil
		}

		if err != nil {
			break
		}
	}

	if sb.Len() > 0 {
		return sb.String(), nil
	}

	return "", fmt.Errorf("range %d-%d not found in %s", start, end, f.Name())
}

func reverseComplement(seq string) string {
	comp := map[byte]byte{
		'A': 'T', 'T': 'A', 'C': 'G', 'G': 'C',
		'a': 't', 't': 'a', 'c': 'g', 'g': 'c',
		'N': 'N', 'n': 'n',
		'R': 'Y', 'Y': 'R', 'S': 'S', 'W': 'W',
		'K': 'M', 'M': 'K', 'B': 'V', 'V': 'B',
		'D': 'H', 'H': 'D',
	}

	runes := []rune(seq)
	result := make([]byte, len(runes))
	for i, r := range runes {
		if c, ok := comp[byte(r)]; ok {
			result[len(runes)-1-i] = c
		} else {
			result[len(runes)-1-i] = byte(r)
		}
	}
	return string(result)
}
