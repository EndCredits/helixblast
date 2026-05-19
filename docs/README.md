# HelixBLAST

Light, modern BLAST web service — single binary, zero external dependencies.

- **BLAST Search**: Web UI for NCBI BLAST+, with job queue, SSE streaming, alignment viewer
- **Multi-Database BLAST**: Search against multiple databases in one submission. Worker runs each DB sequentially, merges and sorts results, tags every hit with its source database. Partial errors per database are collected and displayed.
- **Transcript Lookup**: GFF3-based gene/transcript/CDS coordinate resolution and sequence extraction, backed by local FASTA or Cloudflare Worker + R2. Standalone transcript lookup disabled in the UI when multiple databases are selected (the `/api/v1/transcripts` endpoint always requires a single `db`).
- **Spatial Search**: Chromosome interval lookup. Click a BLAST hit to see overlapping genes and flanking features.
- **Binary Index (mmap)**: GFF3 annotation stored as a memory‑mapped binary with FNV‑1a hash tables. Zero‑decode startup — RSS starts near zero, grows only as query‑touched pages are paged in. Auto‑detected alongside JSON, no config change required.
- **Offline cache**: BLAST results persist in browser IndexedDB (24h TTL). Cache clear resets all active views.

## Quickstart

```bash
# Build server + prepare tool
make build

# Convert JSON GFF3 index to mmap binary (optional — server auto-detects)
./helixblast-prepare --json refseq.index.json.gz --out refseq.index.bin

# Run server
./helixblast --config config.yaml
```

## CLI Tools

| Binary | Source | Purpose |
|--------|--------|---------|
| `helixblast` | `cmd/server` | Full BLAST web server (REST + SSE + embedded frontend) |
| `helixblast-prepare` | `cmd/prepare` | Standalone index builder: JSON → binary, ~3 MB, no BLAST deps |
| `verify` | `cmd/verify` | Builds temp binary from JSON and compares all entries/families/coords/fasta-index for equivalence |

## Documentation

| Document | Content |
|----------|---------|
| [Configuration](configuration.md) | `config.yaml`, `databases.yaml`, resource auto-detection, rationale |
| [API Reference](api.md) | All endpoints, request/response schemas |
| [Transcript Lookup](transcript-lookup.md) | GFF3 index, local vs Worker, region extraction, seeking |
| [Architecture](architecture.md) | Design decisions, binary index format, memory model, concurrency, data flow |

## Changes (2026-05)

| Change | Detail |
|--------|--------|
| Multi-database BLAST | `POST /api/v1/jobs` accepts `dbs: []string`. Worker runs each DB sequentially; errors per DB collected, merged results carry `database` on each Hit. UI: `Select mode="multiple"` with removable Tag chips. |
| Transcript guard | Standalone transcript lookup disabled when more than one database selected. |
| Cache clear | All view states reset on cache clear (`selectedJobId`, `selectedHit`, `transcriptResult`, `spatialResult`). |
| Transcript mode UX | Jobs card hidden when `queryMode === 'transcript'`. |
| Hit-scoped DB resolution | `currentDB`, transcript, and spatial lookups resolve database from `selectedHit?.database`. |
| Results table scroll | `scroll={{ x: 'max-content' }}` — horizontal scroll for all columns. |
| Binary index (mmap) | Zero‑decode GFF3 index: FNV‑1a hash tables + sorted spatial arrays, memory‑mapped. Auto‑detected by the server alongside existing JSON index. |
| `helixblast-prepare` | Standalone binary that converts JSON GFF3 index → mmap binary. No BLAST+ required. |
| `verify` | End‑to‑end verification tool — builds temp binary and compares all records against JSON source. |
| Docker: Debian runtime | Switched from Alpine to `debian:bookworm-slim` + NCBI BLAST+ tarball from FTP. Builder: `golang:1.26.3-trixie`. |
| `split_fasta_v2.sh` | Streams FASTA directly into `gzip` + `gzip -t` verification. |

