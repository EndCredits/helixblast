# Configuration

HelixBLAST uses two YAML files. No environment variables.

## config.yaml

### server

```yaml
server:
  port: 8080
```

The single HTTP port for both API and embedded frontend. No separate dev/prod ports — the Go binary serves everything.

### jobs

```yaml
jobs:
  result_ttl_hours: 24
```

| Field | Default | Why |
|-------|---------|-----|
| `result_ttl_hours` | `24` | How long a terminal job stays queryable in the in-memory registry before pruning; after that its ID resolves to `404`. Results are ephemeral by design — the server never persists them to disk (see below) |

### Why there is no server-side storage layer

Earlier versions shipped a `storage` abstraction (local disk + S3/MinIO backends, presigned URLs, a janitor sweeping expired artifacts). It was built for a plan where BLAST results and genome/index files lived on object storage read directly by the server. That plan is gone: results are delivered over SSE and persisted client-side in IndexedDB, and genome FASTA / GFF3 indexes are read from the local filesystem or served through the **Cloudflare Worker + R2** pipeline (see [Transcript Lookup](transcript-lookup.md)) — byte-range random reads over the S3 API performed too poorly to optimize. The whole subsystem had zero live callers, so it was removed; the only surviving knob is `jobs.result_ttl_hours`, which now governs the in-memory registry alone.

### blast

```yaml
blast:
  path: ""              # BLAST+ directory — empty = look in $PATH
  max_jobs: 20          # Absolute concurrency cap
  cpu_per_job: 2        # Threads per BLAST job
```

| Field | Default | Why |
|-------|---------|-----|
| `path` | `""` | Where to find `blastn`, `blastp`, etc. Empty means search `$PATH` |
| `max_jobs` | `20` (code default; sample files set `5`) | Hard limit on concurrent BLAST processes. Prevents system overload. Actual concurrency is capped further by CPU/memory auto-detection (see below) |
| `cpu_per_job` | `2` | Threads allocated to each BLAST process. Higher = faster per-job, fewer concurrent jobs |

### Resource auto-detection

On startup, HelixBLAST computes actual concurrency:

```
CPU concurrent  = max(1, runtime.NumCPU() / cpu_per_job)
Memory limit    = available RAM < 2GB → 2, < 4GB → 5, else 20
Actual concurrent = min(max_jobs, CPU, memory)
```

If `actual concurrent < 5`, the system enters **degraded mode**: `/health` returns `status: degraded` and the frontend shows a warning banner. This is transparent to users — jobs still run, just with limited throughput.

### database

```yaml
database:
  config_path: ./databases.yaml
  worker_url: ""        # Cloudflare Worker URL for transcript lookup
```

`config_path` points to the database manifest. `worker_url` is optional — set it if using a Cloudflare Worker for transcript-to-genome lookups. When also configured with local `transcript` sections in `databases.yaml`, local takes priority.

## databases.yaml

Each entry declares a BLAST database and optionally a transcript lookup source.

```yaml
databases:
  - name: "arachis-9102"
    type: "protein"
    path: "/path/to/blastdb/YZ9102-prot"
    description: "Peanut YZ9102 genome annotation"
    last_updated: "2026-05-16"
    transcript:
      index_path: "/data/gff3/arachis-9102.index.bin"
      fasta_dir: "/data/genome/arachis-9102/"
      # fasta_file: "/data/genome/arachis-9102.fa"  # alternative: single multi-FASTA
```

| Field | Required | Why |
|-------|----------|-----|
| `name` | Yes | Must match R2 directory name when using Cloudflare Worker transcript lookup |
| `type` | Yes | `nucleotide` or `protein` |
| `path` | Yes | Absolute path to the BLAST database (the prefix before `.phr`/`.pin`/`.psq`) |
| `description` | No | Displayed in the UI dropdown |
| `last_updated` | No | Displayed in the UI |
| `is_chromosome_db` | No | When `true`, automatically resolves BLAST hits to overlapping genes using the spatial index (`/api/v1/spatial`). Use for genome databases built from chromosome sequences |
| `transcript` | No | Enables transcript lookup for this database |
| `transcript.source` | No | For Worker mode: use this name as the `db` parameter instead of `name`. Allows protein and nucleotide BLAST databases to share one set of R2 FASTA files |

### transcript section

| Field | Why |
|-------|-----|
| `index_path` | Path to the preprocessed GFF3 index. **Use `.bin` (memory-mapped binary) unless you have a specific reason for JSON** — build it directly with `helixblast-index --gff3 … --fasta … --out ….bin`. `.json`/`.json.gz` remain supported (decoded once, then cached by path); a `.bin` alongside a configured JSON path is auto-preferred. |
| `fasta_dir` | Directory with per-chromosome FASTA files (`Chr01.fa`, `Chr02.fa`, ...). Tried first |
| `fasta_file` | Single multi-FASTA file containing all chromosomes. Fallback if `fasta_dir` chromosome not found |
| `source` | For Worker mode: if protein and nucleotide BLAST DBs share one genome, set this to a common name so the Worker looks in a single R2 directory |

Multi-database sharing:

```yaml
databases:
  - name: "arachis-9102-prot"
    type: "protein"
    path: "/path/to/blastdb/prot"
    transcript:
      index_path: "/data/gff3/arachis-9102.index.bin"
      source: "arachis-9102"          # Worker: points to R2 fasta/arachis-9102/
  - name: "arachis-9102-nuc"
    type: "nucleotide"
    path: "/path/to/blastdb/nuc"
    transcript:
      index_path: "/data/gff3/arachis-9102.index.bin"
      source: "arachis-9102"          # same shared R2 directory
```

When `source` is set, the Worker request uses that name instead of the BLAST DB name. This avoids uploading the same genome to two R2 directories.

### Hot reload

`databases.yaml` is watched via `fsnotify`. Changes are picked up within seconds — no restart needed. If the new config is invalid (e.g., missing required fields), the old config is kept and an error is logged.

## Generating the transcript index

The GFF3 index is pre-built with `helixblast-index` (Go, replaces the former Node.js `prepare.js`):

```bash
make build-index
./helixblast-index --gff3 input.gff3 --fasta genome.fa --out db-name.index.bin
# optional: --json db-name.index.json.gz also writes the intermediate JSON
```

The command writes a memory‑mapped binary index directly — **this is the recommended runtime format**. With `--json` it additionally produces the intermediate JSON (`db-name.index.json.gz`) for inspection/debugging or the `verify` tool; prefer pointing `index_path` at the `.bin` and leaving JSON out of the runtime path. The Cloudflare Worker does **not** consume the index at all — it only reads FASTA from R2. The index contains:

- **entries** — every GFF3 ID (gene, mRNA, CDS, exon) resolved to its parent mRNA's genomic coordinates via Parent chain traversal
- **families** — gene → transcript/CDS/exon ID lists for inter-query
- **coords** — per-transcript exon/CDS coordinate arrays for sequence extraction
- **fasta_index** — per-chromosome byte offsets in the FASTA file for O(1) seeking
- **spatial** — per-chromosome sorted features for interval lookup

For per-chromosome FASTA files (recommended for Worker), split the genome first:

```bash
./worker/scripts/split_fasta.sh /path/to/genome.fa output_dir/
# → output_dir/Chr01.fa.gz, Chr02.fa.gz, ...
```

### Legacy JSON → bin path (kept for compatibility)

The `helixblast-prepare` tool still converts an existing JSON index to the mmap binary, for setups that already have a JSON index:

```bash
make build-prepare
./helixblast-prepare --json db-name.index.json.gz --out db-name.index.bin
```

Place the `.bin` alongside the existing `.json.gz` — the server auto‑detects it via `LoadIndexAuto()`. No config change required. The binary format carries no Go‑heap resident cost (OS pages in on demand, released on close), whereas the JSON path keeps one cached decoded copy per index (~0.4 KB live heap per entry) after a one‑time decode.
