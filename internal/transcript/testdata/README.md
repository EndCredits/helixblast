# Transcript extraction fixtures

Deterministic FASTA fixtures for characterization tests of the three
`extractRange*` paths. Regenerate with:

```bash
go run ./internal/transcript/testdata/gen
```

## Logical genome (identical content in every file)

| Chrom | Length | Purpose |
|---|---|---|
| Chr01 | 50 000 | cross-path equivalence; long-line case |
| Chr02 | 6 001 | not a multiple of any wrap width → partial final line |
| Chr03 | 250 | tiny record |

Content is a **stateless formula** so tests compute expected substrings directly:

```
base(i) = "ACGT"[(3·i² + 17·i + seed(chr)) % 4]     // i is 1-based
seed(chr) = (sum of name bytes) % 4                 // Chr01=2 Chr02=3 Chr03=0
```

Headers carry a description (`>Chr01 synthetic test chromosome len=… seed=…`)
to pin the "split header on first space" behavior.

## Files → extractor routing

| File | Shape | Routed through |
|---|---|---|
| `multi_wrapped.fa` | 3 records, 60-col wrap | `extractRangeMulti` (targetChr set; with/without fastaIndex seek) |
| `single_wrapped.fa` | 1 record, 70-col wrap | `extractRangeScanner` (isLongLineFASTA=false; width differs from multi on purpose) |
| `single_longline.fa` | 1 record, single 50 000-char line | `extractRangeChunked` (121…65536 detection window) |

## Extraction contract (pinned by `local_test.go`)

- **Format independence**: identical logical content extracts identically from
  all three layouts (multi-record 60-col, single 70-col, single 50 KB line).
- **Out-of-range requests return partial results**, not errors
  (Chr02 5900–6010 → 102 bases, stops at next header).
- `>` at any line start opens a new record (never leaks into bases);
  `\r`/space/tab are ignored anywhere, including inside lines.
- A stale `fasta_index` offset falls back to a linear scan; a final header
  without trailing newline is still recognized.
- Memory is O(64 KB + result) for any line length — an 11 MB single-line
  chromosome extracts fine (regression test `TestExtractRange_ArbitraryLongLine`).
