#!/bin/bash
# split_fasta.sh — Split a multi-FASTA genome into per-chromosome .fa.gz files
set -euo pipefail

GENOME="${1:?Usage: ./split_fasta.sh <genome.fa> [output_dir]}"
OUTDIR="${2:-split_chromosomes}"

mkdir -p "$OUTDIR"

echo "Splitting and compressing on the fly..."

ulimit -n 4096 2>/dev/null || true

awk -v outdir="$OUTDIR" '
/^>/ {
    if (cmd) close(cmd)
    
    header = substr($0, 2)
    split(header, parts, " ")
    chr = parts[1]
    
    cmd = "gzip -c > " outdir "/" chr ".fa.gz"
    
    print $0 | cmd
    next
}
cmd {
    print $0 | cmd
}
END {
    if (cmd) close(cmd)
}
' "$GENOME"

echo "Done! Verifying..."
corrupt=0
for f in "$OUTDIR"/*.fa.gz; do
  gzip -t "$f" || { echo "Corrupt: $f"; corrupt=1; }
done
if [ "$corrupt" -ne 0 ]; then
  echo "Verification failed — some files are corrupt."
  exit 1
fi

count=$(ls "$OUTDIR"/*.fa.gz 2>/dev/null | wc -l | tr -d ' ')
echo "All $count chromosomes OK in $OUTDIR/"
