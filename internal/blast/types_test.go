package blast

import (
	"testing"
)

func TestValidateFASTANucleotide(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid single", ">seq1\nATGCGTACGTAGCTAGCTAGCTAGC", false},
		{"valid multiline", ">seq1 desc here\nATGCGTACGTA\nGCTAGCTAGCTAGC", false},
		{"valid with gaps", ">seq1\nATGC-*TAC", false},
		{"no header", "ATGCGTACGTAGCTAGCTAGCTAGC", true},
		{"empty", "", true},
		{"invalid chars", ">seq1\nATG1CGTACGTAGCTAGCTAGCTAGC", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFASTA(tt.input, "nucleotide")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFASTA() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateFASTAProtein(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid protein", ">prot1\nMKTIIALSYIFCLVFADYKDDDDK", false},
		{"invalid amino acid", ">prot1\nMKTIIAL123SYIFCLVFADYKDDDDK", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFASTA(tt.input, "protein")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFASTA() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeShell(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"normal text", false},
		{"text with semicolon;", true},
		{"text with | pipe", true},
		{"text with & ampersand", true},
		{"text with $ dollar", true},
		{"text with ` backtick", true},
		{"text with \nnewline", true},
		{"word_size", false},
		{"1e-5", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			err := SanitizeShell(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("SanitizeShell(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidProgram(t *testing.T) {
	if !ValidProgram("blastn") {
		t.Error("blastn should be valid")
	}
	if !ValidProgram("blastp") {
		t.Error("blastp should be valid")
	}
	if ValidProgram("nonexistent") {
		t.Error("nonexistent should not be valid")
	}
}
