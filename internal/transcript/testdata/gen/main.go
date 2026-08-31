// Command gen produces the deterministic FASTA fixtures used by the
// transcript extraction characterization tests:
//
//	go run ./internal/transcript/testdata/gen [outdir]
//
// Default outdir is the parent testdata directory. All fixtures encode the
// SAME logical genome (see base()) so the three extractor paths can be
// cross-checked for format-independence.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
)

const alphabet = "ACGT"

type chrom struct {
	name string
	len  int
}

var genome = []chrom{
	{"Chr01", 50000}, // 121..65536 window: single line detectable by isLongLineFASTA → Chunked path.
	// NOTE: lines > 64KB defeat isLongLineFASTA (default scanner token cap) —
	// that detection-failure case is generated on-the-fly in tests.
	{"Chr02", 6001}, // not a multiple of any wrap width: partial final line
	{"Chr03", 250},  // tiny record
}

// seed is a stable per-chromosome offset derived from the name bytes.
func seed(c string) int {
	s := 0
	for i := 0; i < len(c); i++ {
		s += int(c[i])
	}
	return s % 4
}

// base returns the residue at 1-based position i of chromosome c.
// Stateless on purpose: tests can compute expected substrings directly.
func base(c string, i int) byte {
	return alphabet[(3*i*i+17*i+seed(c))%4]
}

func seqOf(c chrom) []byte {
	b := make([]byte, c.len)
	for i := 0; i < c.len; i++ {
		b[i] = base(c.name, i+1)
	}
	return b
}

func writeWrapped(w *bufio.Writer, c chrom, width int) {
	fmt.Fprintf(w, ">%s synthetic test chromosome len=%d seed=%d\n", c.name, c.len, seed(c.name))
	s := seqOf(c)
	for i := 0; i < len(s); i += width {
		end := min(i+width, len(s))
		w.Write(s[i:end])
		w.WriteByte('\n')
	}
}

func writeSingleLine(w *bufio.Writer, c chrom) {
	fmt.Fprintf(w, ">%s synthetic test chromosome len=%d seed=%d\n", c.name, c.len, seed(c.name))
	w.Write(seqOf(c))
	w.WriteByte('\n')
}

func emit(dir, name string, fn func(*bufio.Writer)) {
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	w := bufio.NewWriter(f)
	fn(w)
	if err := w.Flush(); err != nil {
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}
	fmt.Println("wrote", path)
}

func main() {
	dir := "internal/transcript/testdata"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}

	emit(dir, "multi_wrapped.fa", func(w *bufio.Writer) {
		for _, c := range genome {
			writeWrapped(w, c, 60)
		}
	})
	emit(dir, "single_wrapped.fa", func(w *bufio.Writer) {
		writeWrapped(w, genome[0], 70)
	})
	emit(dir, "single_longline.fa", func(w *bufio.Writer) {
		writeSingleLine(w, genome[0])
	})
}
