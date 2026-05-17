# HelixBLAST

Light, modern BLAST web service — single binary, zero external dependencies.

- **BLAST Search**: Web UI for NCBI BLAST+, with job queue, SSE streaming, alignment viewer
- **Multi-Database BLAST**: Search against multiple databases in one submission. Internal worker runs each database sequentially, merges and sorts results by bitscore, and tags every hit with its source database. Partial failures per database are collected and displayed without blocking successful searches.
- **Transcript Lookup**: GFF3-based gene/transcript/CDS coordinate resolution and sequence extraction, backed by local FASTA or Cloudflare Worker + R2. Restricted to single-database mode — cross-genome transcript lookups are blocked.
- **Spatial Search**: Chromosome interval lookup — click a BLAST hit to see overlapping genes, flanking features. Database auto‑detected from the hit's `database` tag.
- **Offline cache**: BLAST results persist in browser IndexedDB across restarts (24h TTL). Clearing the local cache resets all active views (job detail, selected hit, transcript result, spatial result).

## Changes (2026-05)

| Change | Detail |
|--------|--------|
| Multi-database BLAST | `POST /api/v1/jobs` accepts `dbs: []string`. Worker runs each DB sequentially; errors per DB collected, merged results carry `database` on each Hit. UI: `Select mode="multiple"` with removable Tag chips above. |
| Transcript guard | Standalone transcript lookup disabled when more than one database selected. Cross‑genome confusion prevented. |
| Cache clear | `setSelectedJobId(null)`, `setSelectedHit(null)`, `setTranscriptResult(null)`, `setSpatialResult(null)` all reset on cache clear. |
| Transcript mode UX | Jobs card hidden when `queryMode === 'transcript'`. Right panel is Results‑only. |
| Hit-scoped DB resolution | `currentDB`, transcript, and spatial lookups resolve database from `selectedHit?.database` instead of `jobDetail?.database`. |
| Results table scroll | `scroll={{ x: 'max-content' }}` — all columns visible with horizontal scroll in narrow layouts. |
| `split_fasta_v2.sh` | Streams FASTA directly into `gzip`, no intermediate files. Adds `gzip -t` integrity verification. |

```
┌─ HelixBLAST Server (local) ─────────────────────────────────────┐
│  GFF3 index (.json.gz)  →  resolve ID  →  coordinates + family │
│  Local FASTA?  →  f.Seek(fasta_index[chr])  →  O(1) extract    │
│  No local?     →  Worker /sequence  →  extract from R2          │
└──────────────────────────────┬──────────────────────────────────┘
                               │ (optional, when local FASTA absent)
┌──────────────────────────────▼──────────────────────────────────┐
│  Cloudflare Worker (thin I/O pipe)                              │
│  GET /sequence?chr=&start=&end=                                 │
│  R2 get(fasta/db/chr.fa.gz) → DecompressionStream → extract    │
│  R2: fasta/<db>/<chr>.fa.gz only (no index files on R2)        │
└─────────────────────────────────────────────────────────────────┘
```

## Quickstart

```bash
# Build
make build

# Configure
cp config.yaml.example config.yaml
# Edit config.yaml and databases.yaml

# Run
./helixblast --config config.yaml
```

## Documentation

| Document | Content |
|----------|---------|
| [Configuration](configuration.md) | `config.yaml`, `databases.yaml`, resource auto-detection, rationale |
| [API Reference](api.md) | All endpoints, request/response schemas |
| [Transcript Lookup](transcript-lookup.md) | GFF3 index, local vs Worker, region extraction, seeking |
| [Architecture](architecture.md) | Design decisions, data flow, concurrency model, job lifecycle |
