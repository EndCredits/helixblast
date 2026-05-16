package blast

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

func ParseOutfmt6(r io.Reader) ([]Hit, error) {
	scanner := bufio.NewScanner(r)
	hits := make(map[string]*Hit)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 12 {
			continue
		}

		subjectID := fields[1]
		identity, _ := strconv.ParseFloat(fields[2], 64)
		coverage, _ := strconv.ParseFloat(fields[3], 64)
		evalue := fields[4]
		bitscore, _ := strconv.ParseFloat(fields[5], 64)
		qstart, _ := strconv.Atoi(fields[6])
		qend, _ := strconv.Atoi(fields[7])
		sstart, _ := strconv.Atoi(fields[8])
		send, _ := strconv.Atoi(fields[9])
		qseq := fields[10]
		sseq := fields[11]

		hsp := HSP{
			QueryStart:   qstart,
			QueryEnd:     qend,
			SubjectStart: sstart,
			SubjectEnd:   send,
			QuerySeq:     qseq,
			SubjectSeq:   sseq,
		}

		hit, exists := hits[subjectID]
		if !exists {
			hit = &Hit{
				SubjectID:  subjectID,
				Identity:   identity,
				Coverage:   coverage,
				EValue:     evalue,
				TotalScore: bitscore,
				Alignments: make([]HSP, 0),
			}
			hits[subjectID] = hit
		}

		if bitscore > hit.TotalScore {
			hit.TotalScore = bitscore
			hit.Identity = identity
			hit.Coverage = coverage
			hit.EValue = evalue
		}

		hit.Alignments = append(hit.Alignments, hsp)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan blast output: %w", err)
	}

	result := make([]Hit, 0, len(hits))
	for _, h := range hits {
		result = append(result, *h)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalScore > result[j].TotalScore
	})

	if len(result) > 20 {
		result = result[:20]
	}

	return result, nil
}
