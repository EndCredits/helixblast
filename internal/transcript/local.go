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

const (
	extractBufSize  = 64 * 1024
	headerNameLimit = 4 * 1024
)

// extractRange returns bases [start,end] of record targetChr (every residue
// of the first record when targetChr == ""). fastaIndex optionally supplies
// byte offsets of record headers for O(1) positioning; a stale offset falls
// back to a linear scan. Requests past the record end return the available
// partial result. Lines are processed in fixed-size fragments, so memory is
// O(extractBufSize + result) regardless of line length — single-line
// genomes included.
func extractRange(path, targetChr string, start, end int, fastaIndex map[string]int64) (string, error) {
	if targetChr != "" {
		if off, ok := fastaIndex[targetChr]; ok {
			seq, found, err := extractAtOffset(path, off, targetChr, start, end)
			if err != nil {
				return "", err
			}
			if found {
				return seq, nil
			}
			// stale/invalid offset — fall through to linear scan
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	x := newExtractor(targetChr, start, end)
	if err := x.scan(bufio.NewReaderSize(f, extractBufSize), f.Name()); err != nil {
		return "", err
	}
	return x.result(f.Name())
}

// extractAtOffset verifies that a header line sits at the recorded byte
// offset and extracts from there. found=false signals a stale offset.
func extractAtOffset(path string, off int64, targetChr string, start, end int) (string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return "", false, nil
	}
	br := bufio.NewReaderSize(f, extractBufSize)

	name, isHeader, err := readHeaderName(br)
	if err != nil || !isHeader || name != targetChr {
		return "", false, nil
	}

	x := newExtractor(targetChr, start, end)
	x.inTarget = true
	x.seenTarget = true
	if err := x.scan(br, f.Name()); err != nil {
		return "", false, err
	}
	seq, err := x.result(f.Name())
	return seq, true, err
}

// readHeaderName consumes one line (assumed short) and returns the record
// name if the line is a FASTA header.
func readHeaderName(br *bufio.Reader) (string, bool, error) {
	var name strings.Builder
	sawAny := false
	for {
		ch, err := br.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", false, err
		}
		if ch == '\n' {
			break
		}
		if ch == '\r' || ch == '\t' || ch == ' ' {
			if sawAny {
				if ch != '\r' {
					// space/tab ends the name; ignore the rest of the line
					_, _ = br.ReadString('\n')
				}
				break
			}
			continue
		}
		if !sawAny {
			if ch != '>' {
				return "", false, nil // not a header line
			}
			sawAny = true
			continue
		}
		if name.Len() < headerNameLimit {
			name.WriteByte(ch)
		}
	}
	if !sawAny {
		return "", false, nil
	}
	return name.String(), true, nil
}

// extractor is a byte-level FASTA state machine. It never materializes a
// line: fragments are consumed in place.
type extractor struct {
	targetChr string
	start     int
	need      int

	sb         strings.Builder
	pos        int // 1-based residue counter inside the target record
	inTarget   bool
	seenTarget bool

	atLineStart    bool
	inHeader       bool
	headerName     strings.Builder
	headerDone     bool
	headerOverflow bool

	finished bool // stop scanning entirely
}

func newExtractor(targetChr string, start, end int) extractor {
	if start < 1 {
		start = 1
	}
	return extractor{
		targetChr:   targetChr,
		start:       start,
		need:        end - start + 1,
		inTarget:    targetChr == "",
		seenTarget:  targetChr == "",
		atLineStart: true,
	}
}

func (x *extractor) scan(br *bufio.Reader, name string) error {
	for !x.finished {
		frag, err := br.ReadSlice('\n')
		for _, ch := range frag {
			x.feed(ch)
			if x.finished {
				return nil
			}
		}
		switch {
		case err == bufio.ErrBufferFull:
			continue
		case err == io.EOF:
			x.endLine() // final line without trailing newline
			return nil
		case err != nil:
			return fmt.Errorf("read %s: %w", name, err)
		}
	}
	return nil
}

func (x *extractor) feed(ch byte) {
	switch {
	case ch == '\n':
		x.endLine()
		return
	case x.inHeader:
		x.feedHeader(ch)
		return
	case ch == '\r' || ch == ' ' || ch == '\t':
		return
	}

	if x.atLineStart {
		x.atLineStart = false
		if ch == '>' {
			x.inHeader = true
			x.headerName.Reset()
			x.headerDone = false
			x.headerOverflow = false
			return
		}
	}

	if !x.inTarget {
		return
	}
	x.pos++
	if x.pos >= x.start && x.sb.Len() < x.need {
		x.sb.WriteByte(ch)
	}
	if x.sb.Len() >= x.need {
		x.finished = true
	}
}

func (x *extractor) feedHeader(ch byte) {
	if !x.headerDone {
		if ch == ' ' || ch == '\t' {
			x.headerDone = true
			return
		}
		if x.headerName.Len() < headerNameLimit {
			x.headerName.WriteByte(ch)
		} else {
			x.headerOverflow = true
		}
	}
}

func (x *extractor) endLine() {
	if !x.inHeader {
		x.atLineStart = true
		return
	}
	x.inHeader = false
	x.atLineStart = true

	name := ""
	if !x.headerOverflow {
		name = x.headerName.String()
	}

	if x.inTarget {
		if x.targetChr == "" {
			if x.pos > 0 {
				// second record in single-record scope: our record ended
				x.finished = true
				return
			}
			// opening header of the record we extract from
			return
		}
		// leaving the target record — return what we have (partial or full)
		x.finished = true
		return
	}

	if x.targetChr != "" && !x.headerOverflow && name == x.targetChr {
		x.inTarget = true
		x.seenTarget = true
		x.pos = 0
	}
}

func (x *extractor) result(name string) (string, error) {
	if x.targetChr != "" && !x.seenTarget {
		return "", fmt.Errorf("chromosome %s not found in %s", x.targetChr, name)
	}
	if x.sb.Len() == 0 {
		return "", fmt.Errorf("range %d-%d not found in %s", x.start, x.start+x.need-1, name)
	}
	if x.sb.Len() >= x.need {
		return x.sb.String()[:x.need], nil
	}
	return x.sb.String(), nil
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
