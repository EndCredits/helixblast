# Transcript Lookup

The transcript lookup subsystem maps any GFF3 identifier (gene, mRNA, CDS, exon) to genomic coordinates and extracts the corresponding sequence. It supports both local and Cloudflare Worker backends.

## Why this exists

BLAST is for *discovery* — finding similar sequences in a large database. But for known gene models, we already have the exact coordinates from annotation. Running BLAST for a transcript-to-genome lookup is wasteful: it's slow (heuristic search), imprecise (approximate matches), and resource-intensive.

Transcript lookup bypasses BLAST entirely:
1. Resolve the ID to genomic coordinates via a pre-built GFF3 index
2. Extract the sequence range directly from the genome FASTA file

This is **100% precise**, **orders of magnitude faster** than BLAST, and uses **negligible CPU/memory**.

## Architecture

```
User enters ID in HelixBLAST UI
        │
        ▼
GET /api/v1/transcripts?db=X&transcript=Y
        │
        ├─ databases.yaml has transcript.index_path?
        │     └─ Yes → internal/transcript/local.go
        │            Load index.json → resolve ID → f.Seek(chr_offset) → extract range
        │
        └─ config.yaml has database.worker_url?
              └─ Yes → proxy to Cloudflare Worker
                     HelixBLAST loads index.json.gz from local storage → extracts from R2 FASTA
```

## Index format

The index is pre-built from a GFF3 annotation file and the genome FASTA using `worker/scripts/prepare.js`:

```bash
node worker/scripts/prepare.js input.gff3 db-name /path/to/genome.fa
```

Output structure:

```json
{
  "entries": {
    "g1":       { "chr": "Chr01", "start": 11401, "end": 11811, "strand": "+", "type": "gene",  "gene": "g1" },
    "g1.t1":    { "chr": "Chr01", "start": 11401, "end": 11811, "strand": "+", "type": "mRNA",  "gene": "g1" },
    "g1.t1.CDS1": { "chr": "Chr01", "start": 11401, "end": 11811, "strand": "+", "type": "CDS", "gene": "g1" }
  },
  "families": {
    "g1": { "transcripts": ["g1.t1"], "cdss": ["g1.t1.CDS1"], "exons": ["g1.t1.exon1"] }
  },
  "coords": {
    "g1.t1": {
      "exons": [{"start": 11401, "end": 11811}],
      "cdss": [{"start": 11401, "end": 11811}]
    }
  },
  "fasta_index": {
    "Chr01": 0,
    "Chr02": 125678901
  }
}
```

### How parent chain resolution works

GFF3 uses `Parent` attributes to define relationships:
```
gene  ID=g1
mRNA  ID=g1.t1  Parent=g1        → resolves to gene coordinates
CDS   ID=g1.t1.CDS1  Parent=g1.t1 → resolves to mRNA coordinates through chain
```

`prepare.js` walks each entry up the parent chain until it finds a gene or mRNA with genomic coordinates. Every ID in the file maps to its parent mRNA's chromosome and position — so querying by gene, mRNA, CDS, or exon all return the same genomic locus.

### Why `fasta_index` is mandatory

Without byte offsets, finding chromosome 19 in a 2.5GB multi-FASTA file requires scanning through chromosomes 1-18 first — ~1 minute of disk I/O. With `fasta_index`, `f.Seek(chr19_offset, io.SeekStart)` jumps directly to the target chromosome in ~1ms. This is a 60,000x speedup for late chromosomes.

## Local mode (Go)

`internal/transcript/local.go` handles local FASTA files. Three extraction strategies, selected automatically:

| Strategy | When | Speed |
|----------|------|-------|
| `extractRangeMulti` + Seek | `fasta_index` has offset for target chromosome | O(1) — direct jump |
| `extractRangeMulti` + scan | Multi-FASTA without index (fallback) | O(n) — line-by-line header scan |
| `extractRangeScanner` | Single-chromosome file, standard line length | O(chromosome) — line-level skipping |
| `extractRangeChunked` | Single-chromosome file, long lines (>120bp) | O(chromosome) — byte-by-byte streaming |

The scanned sequence covers `(start - 5000)` to `end` to include 5kb upstream. Exon and CDS stitching is done client-side from the returned coordinates.

## Cloudflare Worker mode

`worker/src/index.js` is a thin I/O pipe. It does NOT load or parse the GFF3 index — that stays on the HelixBLAST server to stay under the 10ms Cloudflare Workers CPU limit (free plan).

The Worker only exposes `/sequence`:

```
GET /sequence?db=&chr=&start=&end=&strand=
  → fetch fasta/<db>/<chr>.fa.gz from R2
  → DecompressionStream('gzip') → stream decompress
  → extractRange(start, end) → sliding window, stops when range collected
  → return { sequence: "ATGC..." }
```

Per-chromosome FASTA files only — no multi-FASTA fallback. Split with:

```bash
./worker/scripts/split_fasta.sh /path/to/genome.fa output_dir/
```

The Worker uses a sliding window approach — for standard lines (≤120bp), line-level skipping. For long lines, character-by-character scanning. In both cases, stops reading as soon as the range is collected. `DecompressionStream` is a native API that doesn't count against JS CPU time.

### R2 bucket structure

The Worker only extracts sequences — index files stay on the HelixBLAST server.

```
helixblast-genomes/                  # R2 bucket
└── fasta/
    └── <db-name>/                   # matches databases.yaml `name` field
        ├── Chr01.fa.gz              # per-chromosome compressed FASTA
        ├── Chr02.fa.gz
        └── ...
```

No index files on R2. The HelixBLAST server handles all ID resolution locally (the index is only a few MB compressed).

### Data flow

```
User enters transcript ID in HelixBLAST UI
        │
        ▼
HelixBLAST Server
├─ Load index (local, ~8MB .json.gz) → resolve ID to coordinates
├─ transcript.fasta_dir configured?
│     └─ Yes → extract local (O(1) = f.Seek(fasta_index[chr]))
├─ No, but config.yaml has worker_url?
│     └─ Yes → call Worker /sequence with resolved chr, start, end
│              │
│              ▼
│      Cloudflare Worker (worker/src/index.js)
│      ├─ R2 get("fasta/<db>/<chr>.fa.gz")
│      ├─ DecompressionStream → extractRange(start-5kb, end)
│      └─ return { sequence: "ATGC..." }
│              │
└─ Return: coordinates + sequence + regions + family
```

### Why the index stays local

The full genome index can contain 200K+ entries. Decompressing and JSON-parsing it in a Worker would exceed the 10ms CPU time limit (free plan). The HelixBLAST server has no such constraint — `json.NewDecoder(gzipReader).Decode()` takes <100ms for a 40MB JSON. Worker only handles the I/O-bound sequence extraction from R2.

### Deploying the Worker

```bash
# Prerequisites
npm install -g wrangler

# 1. Configure bucket (edit worker/wrangler.toml)
wrangler r2 bucket create helixblast-genomes

# 2. Upload per-chromosome FASTA
./worker/scripts/split_fasta.sh /path/to/genome.fa output_dir/
for f in output_dir/*.fa.gz; do
    wrangler r2 object put "helixblast-genomes/fasta/<db-name>/$(basename $f)" --file "$f"
done

# 3. Deploy
cd worker
wrangler deploy
# → https://helixblast-gene.<subdomain>.workers.dev

# 4. Configure HelixBLAST
# In config.yaml:
#   database.worker_url: https://helixblast-gene.<subdomain>.workers.dev
# In databases.yaml:
#   transcript.index_path: /data/gff3/db-name.index.json.gz
```

### Worker fetch characteristics

| Aspect | Detail |
|--------|--------|
| R2 fetch latency | ~5-50ms (Cloudflare edge → R2, same region) |
| Decompression | `DecompressionStream('gzip')` — native browser API, streaming |
| Per-chromosome file size | Typically 50-200MB compressed (one chromosome) |
| Memory usage | Only holds the extracted range (~few KB) + one decompression buffer |
| Concurrency | Workers scale horizontally — each request is a new isolate |
| Cold start | ~0ms (no initialization — all logic in request handler) |

The `fasta_index` byte offsets in the local index are NOT used by the Worker (gzip prevents byte-level seeking). Per-chromosome files achieve the same O(1) effect: R2 sends only the requested chromosome's data, skipping all others.

## Region extraction

The genome scan covers 5kb upstream plus the full gene body. Five regions are derived client-side:

1. **5 kb Upstream** (positions 0 to `start - scan_start`): Promoter and upstream regulatory analysis
2. **2 kb Upstream** (last 2kb of upstream): Proximal promoter analysis
3. **Gene Body** (position `start - scan_start` to end): Full gene structure including introns
4. **mRNA** (exons stitched): Mature transcript, UTRs + CDS
5. **CDS** (CDS regions stitched): Coding sequence only

This design does a single genome scan instead of five separate ones. The Worker and local engine only extract one sequence range; the frontend does the slicing.

## Spatial Search

When a BLAST hit lands on a chromosome database (`is_chromosome_db: true`), HelixBLAST automatically resolves the genomic position to overlapping GFF3 features via `/api/v1/spatial`. The spatial index is built alongside the main GFF3 index:

```json
{
  "spatial": {
    "Chr01": [
      { "start": 11401, "end": 11811, "id": "g1", "type": "gene" },
      { "start": 11401, "end": 11811, "id": "g1.t1", "type": "mRNA" },
      { "start": 22075, "end": 27453, "id": "g2", "type": "gene" }
    ]
  }
}
```

Features are sorted by `start` position on each chromosome. The lookup finds all overlapping features plus the nearest upstream and downstream neighbors — so even intergenic hits show flanking genes with distance markers.

This enables a complete workflow: BLAST a sequence → see where it hits on the chromosome → click "Lookup Region" or auto-resolve → see overlapping genes → click any gene/transcript/CDS ID → view its full sequence and exon structure.

## GFF3 coordinate conventions

GFF3 uses 1-indexed, inclusive coordinates. A feature at 11401-11811 spans 411 bases (11811 - 11401 + 1). JavaScript's `slice()` is 0-indexed and exclusive on the end — the frontend adds `+1` to compensate.
