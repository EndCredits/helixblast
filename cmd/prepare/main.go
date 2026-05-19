package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/EndCredits/helixblast/internal/prepare"
)

func main() {
	jsonIdx := flag.String("json", "", "Path to JSON index (.json or .json.gz)")
	output := flag.String("out", "", "Output binary index path")
	flag.Parse()

	if *jsonIdx == "" || *output == "" {
		fmt.Fprintf(os.Stderr, "Usage: helixblast-prepare --json <index.json.gz> --out <index.bin>\n")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Building binary index from %s ...\n", *jsonIdx)
	if err := prepare.BuildBinaryIndex(*jsonIdx, *output); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	info, _ := os.Stat(*output)
	fmt.Fprintf(os.Stderr, "Done: %s (%d bytes)\n", *output, info.Size())
}
