package blast

import (
	"fmt"
	"regexp"
	"strings"
)

type HSP struct {
	QueryStart   int    `json:"query_start"`
	QueryEnd     int    `json:"query_end"`
	SubjectStart int    `json:"subject_start"`
	SubjectEnd   int    `json:"subject_end"`
	QuerySeq     string `json:"query_seq"`
	SubjectSeq   string `json:"subject_seq"`
}

type Hit struct {
	SubjectID  string  `json:"subject_id"`
	Identity   float64 `json:"identity"`
	Coverage   float64 `json:"coverage"`
	EValue     string  `json:"e_value"`
	TotalScore float64 `json:"total_score"`
	Alignments []HSP   `json:"alignments"`
}

type BlastResult struct {
	JobID    string `json:"job_id"`
	Status   string `json:"status"`
	Database string `json:"database"`
	Program  string `json:"program"`
	Results  []Hit  `json:"results"`
}

var validPrograms = map[string]string{
	"blastn":    "nucleotide",
	"blastp":    "protein",
	"blastx":    "protein",
	"tblastn":   "protein",
	"tblastx":   "protein",
	"megablast": "nucleotide",
}

var (
	shellMetaRegexp = regexp.MustCompile(`[;&|$\x60\n\r]`)
	fastaHeaderRe   = regexp.MustCompile(`^>\s*\S+`)
	fastaSeqNucRe   = regexp.MustCompile(`^[ACGTURYKMSWBDHVNacgturykmswbdhvn\-\*]+$`)
	fastaSeqProtRe  = regexp.MustCompile(`^[ACDEFGHIKLMNPQRSTVWYacdefghiklmnpqrstvwy\-\*]+$`)
)

func ValidProgram(p string) bool {
	_, ok := validPrograms[p]
	return ok
}

func ProgramType(p string) string {
	return validPrograms[p]
}

func SanitizeShell(input string) error {
	if shellMetaRegexp.MatchString(input) {
		return fmt.Errorf("input contains prohibited shell characters")
	}
	return nil
}

func ValidateFASTA(fasta string, seqType string) error {
	lines := strings.Split(strings.TrimSpace(fasta), "\n")
	if len(lines) == 0 {
		return fmt.Errorf("empty input")
	}

	hasHeader := false
	seqLines := []string{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ">") {
			if !fastaHeaderRe.MatchString(line) {
				return fmt.Errorf("invalid FASTA header: %s", truncate(line, 60))
			}
			hasHeader = true
			continue
		}
		seqLines = append(seqLines, line)
	}

	if !hasHeader {
		return fmt.Errorf("no FASTA header found")
	}
	if len(seqLines) == 0 {
		return fmt.Errorf("no sequence data found")
	}

	seq := strings.Join(seqLines, "")
	seqUpper := strings.ToUpper(seq)

	switch seqType {
	case "nucleotide":
		if !fastaSeqNucRe.MatchString(seqUpper) {
			return fmt.Errorf("invalid nucleotide sequence characters")
		}
	case "protein":
		if !fastaSeqProtRe.MatchString(seqUpper) {
			return fmt.Errorf("invalid protein sequence characters")
		}
	}

	return nil
}

func SanitizeAdvancedParam(key, value string) error {
	if err := SanitizeShell(key); err != nil {
		return fmt.Errorf("param key: %w", err)
	}
	if err := SanitizeShell(value); err != nil {
		return fmt.Errorf("param value: %w", err)
	}
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
