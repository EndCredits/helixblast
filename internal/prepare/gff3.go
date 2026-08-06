package prepare

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/EndCredits/gff3-go"
	"github.com/EndCredits/helixblast/internal/transcript"
)

// GFF3 index builder — Go replacement for worker/scripts/prepare.js.
//
// It parses a GFF3 annotation file plus a genome FASTA and produces the same
// GFF3Data structure that the binary index is built from. Semantics mirror the
// original prepare.js:
//
//   - entries: every ID (gene/mRNA/CDS/exon) resolved up the Parent chain to
//     the nearest gene/mRNA/transcript coordinates
//   - gene: the top of the Parent chain for each ID
//   - families: gene → transcript/CDS/exon ID lists
//   - coords: per-transcript (mRNA) exon/CDS coordinate arrays
//   - spatial: per-chromosome sorted features (gene/mRNA/CDS/exon only)
//   - fasta_index: per-chromosome byte offsets in the genome FASTA
//   - Name attribute aliases: when Name != ID, an extra entries key maps Name
//     to the same record (BLAST databases often use the Name as seqid)

type gff3Row struct {
	id       string
	parent   string
	name     string
	typ      string
	chr      string
	start    int
	end      int
	strand   string
}

// BuildGFF3Data parses a GFF3 file and genome FASTA into an in-memory
// GFF3Data, mirroring prepare.js semantics exactly.
func BuildGFF3Data(gff3Path, fastaPath string) (*transcript.GFF3Data, error) {
	rows, err := parseGFF3(gff3Path)
	if err != nil {
		return nil, err
	}

	coordMap := make(map[string]transcript.GFF3Entry) // gene/mRNA/transcript → coords
	parentMap := make(map[string]string)              // id → parent
	nameAlias := make(map[string]string)              // name → id

	for _, r := range rows {
		if r.parent != "" {
			parentMap[r.id] = r.parent
		}
		if r.name != "" {
			nameAlias[r.name] = r.id
		}
		if r.typ == "gene" || r.typ == "mRNA" || r.typ == "transcript" {
			coordMap[r.id] = transcript.GFF3Entry{
				Chr: r.chr, Start: r.start, End: r.end, Strand: r.strand,
			}
		}
	}

	resolveCoords := func(id string) *transcript.GFF3Entry {
		if c, ok := coordMap[id]; ok {
			return &c
		}
		cur := id
		for {
			p, ok := parentMap[cur]
			if !ok {
				return nil
			}
			if c, ok := coordMap[p]; ok {
				return &c
			}
			cur = p
		}
	}

	resolveGene := func(id string) string {
		p, ok := parentMap[id]
		if !ok {
			return id
		}
		cur := p
		for {
			next, ok := parentMap[cur]
			if !ok {
				return cur
			}
			cur = next
		}
	}

	entries := make(transcript.GFF3Index)
	families := make(transcript.GFF3Families)
	coords := make(transcript.GFF3Coords)

	for _, r := range rows {
		c := resolveCoords(r.id)
		if c == nil {
			continue
		}
		gene := resolveGene(r.id)

		entry := transcript.GFF3Entry{
			Chr: c.Chr, Start: c.Start, End: c.End, Strand: c.Strand,
			Type: r.typ, Gene: gene,
		}
		entries[r.id] = entry
		if r.name != "" && r.name != r.id {
			entries[r.name] = entry
		}

		if _, ok := families[gene]; !ok {
			families[gene] = transcript.GFF3Family{}
		}
		fam := families[gene]

		switch r.typ {
		case "mRNA", "transcript":
			fam.Transcripts = appendUnique(fam.Transcripts, r.id)
			if _, ok := coords[r.id]; !ok {
				coords[r.id] = transcript.TranscriptRegions{}
			}
		case "CDS":
			fam.CDSs = appendUnique(fam.CDSs, r.id)
			if r.parent != "" {
				tr := coords[r.parent]
				tr.CDSs = append(tr.CDSs, transcript.RegionCoord{Start: r.start, End: r.end})
				coords[r.parent] = tr
			}
		case "exon":
			fam.Exons = appendUnique(fam.Exons, r.id)
			if r.parent != "" {
				tr := coords[r.parent]
				tr.Exons = append(tr.Exons, transcript.RegionCoord{Start: r.start, End: r.end})
				coords[r.parent] = tr
			}
		}

		families[gene] = fam
	}

	for id := range coords {
		sort.Slice(coords[id].Exons, func(i, j int) bool { return coords[id].Exons[i].Start < coords[id].Exons[j].Start })
		sort.Slice(coords[id].CDSs, func(i, j int) bool { return coords[id].CDSs[i].Start < coords[id].CDSs[j].Start })
	}

	spatial := buildSpatial(rows)

	fastaIndex, err := buildFastaIndex(fastaPath)
	if err != nil {
		return nil, err
	}

	return &transcript.GFF3Data{
		Entries:    entries,
		Families:   families,
		Coords:     coords,
		Spatial:    spatial,
		FastaIndex: fastaIndex,
	}, nil
}

func parseGFF3(path string) ([]gff3Row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open gff3: %w", err)
	}
	defer f.Close()

	r := gff3.NewReader(f)
	var rows []gff3Row
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse gff3: %w", err)
		}
		id := rec.Attributes.Get("ID")
		if id == "" {
			continue
		}
		rows = append(rows, gff3Row{
			id:     id,
			parent: rec.Attributes.Get("Parent"),
			name:   rec.Attributes.Get("Name"),
			typ:    rec.Type,
			chr:    rec.SeqID,
			start:  rec.Start,
			end:    rec.End,
			strand: rec.Strand,
		})
	}
	return rows, nil
}

// buildSpatial keeps only gene/mRNA/CDS/exon features, sorted by start per
// chromosome — same filtering as prepare.js.
func buildSpatial(rows []gff3Row) transcript.GFF3Spatial {
	spatial := make(transcript.GFF3Spatial)
	for _, r := range rows {
		switch r.typ {
		case "gene", "mRNA", "CDS", "exon":
		default:
			continue
		}
		if r.chr == "" {
			continue
		}
		spatial[r.chr] = append(spatial[r.chr], transcript.SpatialFeature{
			Start: r.start, End: r.end, ID: r.id, Type: r.typ,
		})
	}
	for chr := range spatial {
		sort.Slice(spatial[chr], func(i, j int) bool {
			return spatial[chr][i].Start < spatial[chr][j].Start
		})
	}
	return spatial
}

// buildFastaIndex scans the genome FASTA and records the byte offset of each
// chromosome header ('>'), 0-based. Mirrors prepare.js buildFastaIndex.
func buildFastaIndex(path string) (map[string]int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open fasta: %w", err)
	}
	defer f.Close()

	index := make(map[string]int64)
	reader := bufio.NewReaderSize(f, 1*1024*1024)
	var offset int64
	var leftover string

	for {
		chunk := make([]byte, 1*1024*1024)
		n, err := reader.Read(chunk)
		if n == 0 && err != nil {
			break
		}
		text := leftover + string(chunk[:n])
		lines := strings.Split(text, "\n")
		leftover = lines[len(lines)-1]

		for _, line := range lines[:len(lines)-1] {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, ">") {
				header := strings.TrimPrefix(trimmed, ">")
				header = strings.Fields(header)[0]
				index[header] = offset
			}
			offset += int64(len(line)) + 1
		}

		if err != nil {
			break
		}
	}

	return index, nil
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
