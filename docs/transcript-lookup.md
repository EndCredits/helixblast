# Transcript lookup

Transcript lookup resolves a GFF3 identifier to genomic coordinates and extracts the corresponding sequence. It uses the annotation's exact coordinates instead of running a BLAST search. The lookup works with a local FASTA file or with a Cloudflare Worker that reads FASTA from R2.

## How it works

```text
User enters an ID
  -> GET /api/v1/transcripts?db=<name>&transcript=<id>
  -> Load the local GFF3 index from transcript.index_path
  -> Resolve the ID to chromosome, start, end, and strand
  -> Extract the sequence:
       local fasta_dir or fasta_file  -> read that file
       no local FASTA + worker_url    -> GET <worker_url>/sequence
  -> Return coordinates, sequence, exon/CDS regions, and the gene family
```

The GFF3 index is always local. The Worker only returns bases; it does not parse GFF3 or serve an index. A Cloudflare Worker deployment needs a local index on the HelixBLAST host.

## Index format

Build the index directly from GFF3 and genome FASTA:

```bash
./helixblast-index --gff3 input.gff3 --fasta genome.fa --out db-name.index.bin
```

`--out` writes the binary `HXBI` format, version 2. Add `--json` if you also want the intermediate JSON:

```bash
./helixblast-index --gff3 input.gff3 --fasta genome.fa \
  --out db-name.index.bin --json db-name.index.json.gz
```

The JSON form, when written, has this shape:

```json
{
  "entries": {
    "g1": {
      "chr": "Chr01",
      "start": 11401,
      "end": 11811,
      "strand": "+",
      "type": "gene",
      "gene": "g1"
    },
    "g1.t1": {
      "chr": "Chr01",
      "start": 11401,
      "end": 11811,
      "strand": "+",
      "type": "mRNA",
      "gene": "g1"
    },
    "g1.t1.CDS1": {
      "chr": "Chr01",
      "start": 11401,
      "end": 11811,
      "strand": "+",
      "type": "CDS",
      "gene": "g1"
    }
  },
  "families": {
    "g1": {
      "transcripts": ["g1.t1"],
      "cdss": ["g1.t1.CDS1"],
      "exons": ["g1.t1.exon1"]
    }
  },
  "coords": {
    "g1.t1": {
      "exons": [{ "start": 11401, "end": 11811 }],
      "cdss": [{ "start": 11401, "end": 11811 }]
    }
  },
  "fasta_index": {
    "Chr01": 0,
    "Chr02": 125678901
  },
  "spatial": {
    "Chr01": [
      { "start": 11401, "end": 11811, "id": "g1", "type": "gene" }
    ]
  }
}
```

The binary index stores the same data as packed records:

| Section | Content |
|---|---|
| `entries` | Every GFF3 ID whose Parent chain reaches a gene, mRNA, or transcript, plus every `Name` alias. Gene, mRNA, and transcript rows use their own coordinates. Child features use the nearest annotated ancestor. |
| `families` | Gene to transcript, CDS, and exon ID lists. |
| `coords` | Exon and CDS coordinate arrays per transcript. |
| `fasta_index` | Byte offset of each chromosome header in the genome FASTA. |
| `spatial` | Per-chromosome features sorted by start. The index stores gene, mRNA, CDS, and exon records; the spatial API returns genes only. |

### Parent chain resolution

GFF3 uses `Parent` attributes:

```text
gene  ID=g1
mRNA  ID=g1.t1       Parent=g1
CDS   ID=g1.t1.CDS1  Parent=g1.t1
```

The builder resolves a child's coordinates to the nearest gene, mRNA, or transcript ancestor. A gene, mRNA, or transcript row uses its own coordinates, so querying a gene and one of its transcripts can return different spans. A row whose Parent chain does not resolve is not indexed. Multi-parent values such as `Parent=t1,t2` are not split and do not resolve.

`Name` aliases are added to `entries` only. They are not added to `coords` or `families`, so looking up an alias returns coordinates but may not return region data for that alias.

### Why `fasta_index` matters

A multi-FASTA file can be several gigabytes. Without byte offsets, finding a chromosome near the end means scanning every earlier record. With `fasta_index`, the extractor seeks directly to the chromosome header, verifies the name, and reads from there. A stale or wrong offset falls back to a linear scan, so the index affects speed, not correctness.

`fasta_dir` mode does not use `fasta_index`. It opens one file per chromosome and scans from the start of that file.

## Local extraction

`internal/transcript/local.go` handles both local layouts.

| Source | File names | Positioning | Notes |
|---|---|---|---|
| `fasta_dir` | `<chromosome>.fa` | Linear scan from byte 0 | Plain text only. The server does not read `.fa.gz`, `.fasta`, or `.fna` here. |
| `fasta_file` | Any multi-FASTA path | `fasta_index` offset when available, otherwise linear scan | One file contains every chromosome. |

The extractor never holds a whole line in memory. It reads fixed-size fragments, so a single-line chromosome uses about the same memory as a wrapped file: 64 KB plus the requested sequence.

Behavior:

- Sequence lines ignore `\r`, spaces, and tabs.
- A `>` at the start of a line opens a new record. The record name ends at the first space or tab.
- A requested range past the end of a record returns the available partial sequence.
- A range that starts past the last base of a record returns an error instead of an empty string.
- A `fasta_dir` file is treated as a single record. A second header ends extraction.
- A trailing `\r` in the header is treated as a terminator on both the offset and linear paths, so a CRLF FASTA file matches the chromosome name without a `fasta_index` offset.
- For a minus-strand feature, the server reverse-complements the extracted window.

The extraction window depends on the strand:

- Plus strand: `[max(1, start - 5000), end]`.
- Minus strand: `[start, end + 5000]`.

For a minus-strand feature, the high-coordinate side is the 5' side, so the window includes the 5 kb upstream flank. At a chromosome boundary, `scan_end` is the last base actually extracted.

## Cloudflare Worker mode

`worker/src/index.js` is a thin I/O layer. It does not load or parse the GFF3 index. That keeps the request under the Cloudflare Workers CPU limit on the free plan.

Endpoints:

```http
GET /sequence?db=<name>&chr=<chromosome>&start=<n>&end=<n>&strand=<+|->
```

```json
{
  "database": "arachis-9102",
  "chromosome": "Chr01",
  "start": 6401,
  "end": 27453,
  "strand": "+",
  "sequence": "ATGC..."
}
```

```http
GET /health
```

```json
{
  "status": "ok",
  "worker": "helixblast-gene"
}
```

`/sequence` returns `400` when `db`, `chr`, `start`, or `end` is missing or not a number. Extraction failures return `500`. The HelixBLAST server treats a non-200 Worker response as `502 Bad Gateway`.

The Worker supports two FASTA layouts:

1. Per-chromosome files at `fasta/<db>/<chr>.fa.gz`. This is the primary layout.
2. A single multi-FASTA fallback at `fasta/<db>/genome.fa.gz`. The Worker uses it when the per-chromosome object does not exist, then scans for the target chromosome header.

The Worker streams the gzip object through `DecompressionStream`, then extracts the requested range. It stops reading once the range is complete. `DecompressionStream` is native and does not count against JavaScript CPU time.

Each per-chromosome object should contain one record. The Worker does not stop at a second record in that mode.

### R2 bucket layout

```text
helixblast-genomes/
  fasta/
    <db-name>/
      Chr01.fa.gz
      Chr02.fa.gz
      ...
```

The `<db-name>` is `transcript.source` when set, otherwise the database `name`. With `source`, a protein and a nucleotide BLAST database can share one genome directory:

```yaml
transcript:
  index_path: "/data/gff3/arachis-9102.index.bin"
  source: "arachis-9102"
```

The Worker reads `fasta/arachis-9102/<chr>.fa.gz` for both entries.

### Data flow

```text
User enters an ID
  -> HelixBLAST server
  -> Load local index; resolve ID to coordinates
  -> If fasta_dir or fasta_file is configured:
       extract locally
     Else if worker_url is configured:
       GET <worker_url>/sequence?db=&chr=&start=&end=&strand=
       <- { sequence }
  -> Return coordinates + sequence + regions + family
```

### Why the index stays local

A full index can contain hundreds of thousands of entries. Decompressing and parsing it inside a Worker would exceed the free-plan CPU limit. The HelixBLAST host caches the decoded JSON reader by path and reuses it across requests; binary indexes are memory-mapped per request. The Worker only handles the I/O-bound part: reading compressed FASTA from R2 and slicing out one range.

### Deploy the Worker

```bash
# Prerequisites
npm install -g wrangler

# 1. Create the bucket and edit worker/wrangler.toml.
wrangler r2 bucket create helixblast-genomes

# 2. Split the genome and upload one object per chromosome.
./worker/scripts/split_fasta.sh /path/to/genome.fa output_dir/
for f in output_dir/*.fa.gz; do
  wrangler r2 object put "helixblast-genomes/fasta/<db-name>/$(basename "$f")" --file "$f"
done

# 3. Deploy.
cd worker
wrangler deploy
# https://helixblast-gene.<subdomain>.workers.dev

# 4. Configure HelixBLAST.
# config.yaml:
#   database.worker_url: https://helixblast-gene.<subdomain>.workers.dev
# databases.yaml:
#   transcript.index_path: /data/gff3/db-name.index.bin
```

The script streams each record through `gzip` and checks every output with `gzip -t`. It writes compressed files for R2. Do not point a local `fasta_dir` at that output; local mode expects uncompressed `<chromosome>.fa`.

### Worker characteristics

| Aspect | Behavior |
|---|---|
| R2 fetch | Same-region reads, typically tens of milliseconds |
| Decompression | Native `DecompressionStream('gzip')`, streaming |
| Memory | The extracted range plus one decompression buffer |
| Concurrency | One isolate per request, scaled by Cloudflare |
| Cold start | No initialization work in the request handler |

`fasta_index` offsets are not used by the Worker. Gzip prevents byte-level seeking. One object per chromosome gives the same effect: R2 sends only the requested chromosome.

## Fallback boundaries

The sequence source order is:

1. `fasta_dir`
2. `fasta_file`
3. `worker_url`

The Worker is a deployment-mode fallback, not an error fallback. It is used only when both local FASTA fields are empty and `worker_url` is set.

- If `fasta_dir` is configured and `<chromosome>.fa` is missing, the server tries `fasta_file` when that is configured.
- If `fasta_dir` is the only source and the file is missing, the response is `200` with an empty `sequence`. The Worker is not called.
- If `fasta_file` is configured and the chromosome is absent, the lookup returns `404`.
- If `transcript.index_path` is empty, the endpoint returns `503` before any sequence source is considered.
- If the Worker returns an error, the endpoint returns `502`.

## Region extraction

The server returns one sequence window per lookup. The frontend slices five regions from that string:

1. 5 kb upstream
2. 2 kb upstream
3. Gene body
4. mRNA, with exons stitched
5. CDS, with coding regions stitched

For plus-strand features, the first `start - scan_start` bases are the 5 kb upstream region, and the gene body follows.

For minus-strand features, the server reverse-complements the window. The first `scan_end - end` bases are the 5 kb upstream region, and the gene body follows in transcript orientation. The numeric `scan_start` and `scan_end` remain genomic coordinates.

The frontend splices exons and CDS in transcript order. For plus-strand features that is ascending genomic start; for minus-strand features it is descending genomic start.

## Spatial search

When a BLAST hit belongs to a chromosome database (`is_chromosome_db: true`), the frontend sends the hit's range to `/api/v1/spatial`. It uses the hit's `subject_id` as the `chr` value, so the BLAST database must use chromosome names that match the GFF3 `seqid` values. The spatial index is built with the main GFF3 index.

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

Features are sorted by start on each chromosome. The query is gene-centric:

- `overlapping` lists genes whose span intersects the range.
- `upstream` is the nearest gene that ends before the range.
- `downstream` is the nearest gene that starts after the range.
- mRNA, CDS, and exon records are excluded from the response.

Every returned gene keeps its full indexed span. A hit that covers only one exon still reports the gene's complete coordinates. `upstream` and `downstream` are independent of `overlapping`; they describe the nearest genes outside the range and are returned even when the range overlaps other genes.

The reader binary-searches the start-sorted array, scans backward within an 8 Mb window for overlaps and the upstream gene, and reads forward for the downstream gene. A gene longer than 8 Mb could be missed. The API rejects windows wider than 1 Mb with `400`.

The frontend starts with the first HSP in the BLAST output and adds later HSPs that fall within 20 kb of the current span. Each added HSP can extend the span, so a chain of HSPs can grow the window; the 1 Mb cap bounds it. This prevents a low-complexity query with noise HSPs megabases apart from producing a chromosome-scale query.

The result enables this workflow: BLAST a sequence, see where it lands on the chromosome, open the overlapping gene or a flanking gene, and inspect its transcript and exon structure.

## GFF3 coordinate conventions

GFF3 uses 1-indexed, inclusive coordinates. A feature from 11401 to 11811 spans 411 bases. JavaScript `slice()` is 0-indexed and excludes the end, so the frontend adds 1 to the end coordinate when it slices a region.
