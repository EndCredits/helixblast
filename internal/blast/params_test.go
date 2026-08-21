package blast

import (
	"os"
	"path/filepath"
	"testing"
)

const fakeHelpOutput = `Usage: blastn [options]
blastn arguments:
  -query <String>
    Input file name
  -db <String>
    BLAST database name
  -out <String>
    Output file name
  -outfmt <String>
    alignment view options
  -num_threads <Integer, >=1>
    Number of threads
  -remote <Boolean>
    Execute remotely
  -evalue <Real>
    Expectation value
  -word_size <Integer, >=4>
    Word size
  -task <String>
    Task to execute
  -dust <String>
    Filter query sequence with DUST
  -max_hsps <Integer, >=1>
    Set maximum number of HSPs per subject
`

func writeFakeBlastBinary(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\ncat <<'EOF'\n" + fakeHelpOutput + "EOF\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestBuildWhitelist(t *testing.T) {
	dir := t.TempDir()
	for _, prog := range []string{"blastn", "blastp", "blastx", "tblastn", "tblastx"} {
		writeFakeBlastBinary(t, dir, prog)
	}

	wl, err := BuildWhitelist(dir)
	if err != nil {
		t.Fatalf("BuildWhitelist() error = %v", err)
	}

	for _, p := range []string{"evalue", "word_size", "task", "dust", "max_hsps", "remote"} {
		if !wl.IsAllowed(p) {
			t.Errorf("-%s is supported by BLAST+ and should be adjustable", p)
		}
	}

	for _, p := range []string{"db", "query", "outfmt", "num_threads", "out"} {
		if wl.IsAllowed(p) {
			t.Errorf("-%s is server-reserved and must not be overridable via advanced_params", p)
		}
	}

	if wl.IsAllowed("totally_bogus") {
		t.Error("param absent from BLAST+ help output must be rejected")
	}
}

func TestNewParamWhitelist(t *testing.T) {
	wl := NewParamWhitelist([]string{"task", "evalue"})
	if !wl.IsAllowed("task") || !wl.IsAllowed("evalue") {
		t.Error("constructed keys should be allowed")
	}
	if wl.IsAllowed("db") {
		t.Error("unlisted keys should be rejected")
	}
	if wl.Len() != 2 {
		t.Errorf("Len() = %d, want 2", wl.Len())
	}
}
