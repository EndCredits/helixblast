#!/bin/bash
# split_fasta.sh — Split a multi-FASTA genome into per-chromosome .fa.gz files
#
# Usage: ./split_fasta.sh <genome.fa> [output_dir]
#
# Output:
#   output_dir/Chr01.fa.gz
#   output_dir/Chr02.fa.gz
#   ...

set -euo pipefail

GENOME="${1:?Usage: ./split_fasta.sh <genome.fa> [output_dir]}"
OUTDIR="${2:-split_chromosomes}"

mkdir -p "$OUTDIR"

awk -v outdir="$OUTDIR" '
/^>/ {
    if (file) close(file)
    header = substr($0, 2)
    split(header, parts, " ")
    chr = parts[1]
    file = outdir "/" chr ".fa"
    print $0 > file
    next
}
file {
    print $0 > file
}
' "$GENOME"

echo "Splitting complete. Compressing..."
count=0
for fa in "$OUTDIR"/*.fa; do
    [ -f "$fa" ] || continue
    gzip -f "$fa"
    count=$((count + 1))
done

echo "Done: $count chromosome(s) written to $OUTDIR/"
ls -lh "$OUTDIR"/
