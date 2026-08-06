package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/EndCredits/helixblast/internal/prepare"
)

func main() {
	gff3Path := flag.String("gff3", "", "Path to GFF3 annotation file")
	fastaPath := flag.String("fasta", "", "Path to genome FASTA file")
	output := flag.String("out", "", "Output binary index path (.bin)")
	jsonOut := flag.String("json", "", "Optional: also write the intermediate JSON index to this path (.json or .json.gz)")
	flag.Parse()

	if *gff3Path == "" || *fastaPath == "" || *output == "" {
		fmt.Fprintf(os.Stderr, "Usage: helixblast-index --gff3 <annotations.gff3> --fasta <genome.fa> --out <index.bin> [--json <index.json.gz>]\n")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Building GFF3 index from %s + %s ...\n", *gff3Path, *fastaPath)
	data, err := prepare.BuildGFF3Data(*gff3Path, *fastaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building GFF3 data: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "  entries=%d families=%d coords=%d spatial_chr=%d fasta_chr=%d\n",
		len(data.Entries), len(data.Families), len(data.Coords), len(data.Spatial), len(data.FastaIndex))

	if *jsonOut != "" {
		if err := writeJSONIndex(data, *jsonOut); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON index: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Wrote JSON index: %s\n", *jsonOut)
	}

	fmt.Fprintf(os.Stderr, "Writing binary index: %s ...\n", *output)
	if err := prepare.BuildBinaryIndexFromData(data, *output); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing binary index: %v\n", err)
		os.Exit(1)
	}

	info, _ := os.Stat(*output)
	fmt.Fprintf(os.Stderr, "Done: %s (%d bytes)\n", *output, info.Size())
}

func writeJSONIndex(data interface{}, path string) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	defer f.Close()

	if len(path) >= 3 && path[len(path)-3:] == ".gz" {
		w := gzip.NewWriter(f)
		if _, err := w.Write(raw); err != nil {
			return fmt.Errorf("gzip write: %w", err)
		}
		if err := w.Close(); err != nil {
			return fmt.Errorf("gzip close: %w", err)
		}
		return nil
	}

	if _, err := f.Write(raw); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}
