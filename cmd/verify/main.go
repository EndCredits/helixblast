package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/EndCredits/helixblast/internal/prepare"
	"github.com/EndCredits/helixblast/internal/transcript"
)

func main() {
	jsonPath := flag.String("json", "", "Path to JSON index")
	flag.Parse()

	if *jsonPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: verify --json index.json.gz")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Building binary from %s ...\n", *jsonPath)
	binPath := *jsonPath + ".verify.bin"
	if err := prepare.BuildBinaryIndex(*jsonPath, binPath); err != nil {
		fmt.Fprintf(os.Stderr, "Build failed: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(binPath)

	jsonIdx, err := transcript.LoadIndex(*jsonPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load JSON: %v\n", err)
		os.Exit(1)
	}

	binIdx, err := transcript.LoadIndexAuto(binPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load binary: %v\n", err)
		os.Exit(1)
	}
	defer binIdx.Close()

	failures := 0

	// Compare entries
	ids := make([]string, 0, len(jsonIdx.Entries))
	for id := range jsonIdx.Entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		je := jsonIdx.Entries[id]
		be, ok := binIdx.LookupEntry(id)
		if !ok {
			fmt.Printf("MISS  entry  %s (binary missing)\n", id)
			failures++
			continue
		}
		if je.Chr != be.Chr || je.Start != be.Start || je.End != be.End || je.Strand != be.Strand || je.Type != be.Type || je.Gene != be.Gene {
			fmt.Printf("DIFF  entry  %s: json={%s %d-%d %s %s %s} bin={%s %d-%d %s %s %s}\n",
				id, je.Chr, je.Start, je.End, je.Strand, je.Type, je.Gene,
				be.Chr, be.Start, be.End, be.Strand, be.Type, be.Gene)
			failures++
		}
	}

	// Compare families
	for gene, jf := range jsonIdx.Families {
		bf, ok := binIdx.LookupFamily(gene)
		if !ok {
			fmt.Printf("MISS  family %s (binary missing)\n", gene)
			failures++
			continue
		}
		if !strSliceEq(jf.Transcripts, bf.Transcripts) || !strSliceEq(jf.CDSs, bf.CDSs) || !strSliceEq(jf.Exons, bf.Exons) {
			fmt.Printf("DIFF  family %s\n", gene)
			failures++
		}
	}

	// Compare coords
	for id, jc := range jsonIdx.Coords {
		bc, ok := binIdx.LookupCoords(id)
		if !ok {
			fmt.Printf("MISS  coords %s (binary missing)\n", id)
			failures++
			continue
		}
		if len(jc.Exons) != len(bc.Exons) || len(jc.CDSs) != len(bc.CDSs) {
			fmt.Printf("DIFF  coords %s (exon/cds count)\n", id)
			failures++
			continue
		}
		for i := range jc.Exons {
			if jc.Exons[i].Start != bc.Exons[i].Start || jc.Exons[i].End != bc.Exons[i].End {
				fmt.Printf("DIFF  coords %s exon[%d]\n", id, i)
				failures++
			}
		}
		for i := range jc.CDSs {
			if jc.CDSs[i].Start != bc.CDSs[i].Start || jc.CDSs[i].End != bc.CDSs[i].End {
				fmt.Printf("DIFF  coords %s cds[%d]\n", id, i)
				failures++
			}
		}
	}

	// Compare fasta index
	for chr, jo := range jsonIdx.FastaIndex {
		bo, ok := binIdx.FastaOffset(chr)
		if !ok {
			fmt.Printf("MISS  fasta  %s (binary missing)\n", chr)
			failures++
			continue
		}
		if jo != bo {
			fmt.Printf("DIFF  fasta  %s: json=%d bin=%d\n", chr, jo, bo)
			failures++
		}
	}

	if failures == 0 {
		fmt.Println("VERIFIED: JSON and binary indices produce identical results.")
	} else {
		fmt.Printf("FAILED: %d mismatches found.\n", failures)
		os.Exit(1)
	}
}

func strSliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
