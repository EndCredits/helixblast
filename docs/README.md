# HelixBLAST

Light, modern BLAST web service — single binary, zero external dependencies.

- **BLAST Search**: Web UI for NCBI BLAST+, with job queue, SSE streaming, alignment viewer
- **Multi-Database BLAST**: Search against multiple databases in one submission. Worker runs each DB sequentially, merges and sorts results, tags every hit with its source database. Partial errors per database are collected and displayed.
- **Transcript Lookup**: GFF3-based gene/transcript/CDS coordinate resolution and sequence extraction, backed by local FASTA or Cloudflare Worker + R2. Standalone transcript lookup disabled in the UI when multiple databases are selected (the `/api/v1/transcripts` endpoint always requires a single `db`).
- **Spatial Search**: Chromosome interval lookup. Click a BLAST hit to see overlapping genes and flanking features.
- **Binary Index (mmap)**: GFF3 annotation stored as a memory‑mapped binary with xxh3 hash tables. Zero‑decode startup — RSS starts near zero, grows only as query‑touched pages are paged in. Auto‑detected alongside JSON, no config change required.
- **Offline cache**: BLAST results persist in browser IndexedDB (24h TTL). The job list and all results live exclusively in IndexedDB — the server holds no results after SSE delivery. Each browser is an isolated workspace (no cross-device job sharing).

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
| `helixblast-index` | `cmd/indexer` | GFF3 + genome FASTA → binary index (replaces Node.js `prepare.js`) |
| `helixblast-prepare` | `cmd/prepare` | Standalone index builder: JSON → binary, ~3 MB, no BLAST deps |
| `verify` | `cmd/verify` | Builds temp binary from JSON and compares all entries/families/coords/fasta-index for equivalence |

## Documentation

| Document | Content |
|----------|---------|
| [Configuration](configuration.md) | `config.yaml`, `databases.yaml`, resource auto-detection, rationale |
| [API Reference](api.md) | All endpoints, request/response schemas |
| [Transcript Lookup](transcript-lookup.md) | GFF3 index, local vs Worker, region extraction, seeking |
| [Architecture](architecture.md) | Design decisions, binary index format, memory model, concurrency, data flow |

## License

All code in this project is licensed under the MIT License, and the documentation (including the README) is licensed under the CC BY-SA 4.0 License.

## Changes (2026-05)

| Change | Detail |
|--------|--------|
| Multi-database BLAST | `POST /api/v1/jobs` accepts `dbs: []string`. Worker runs each DB sequentially; errors per DB collected, merged results carry `database` on each Hit. UI: `Select mode="multiple"` with removable Tag chips. |
| Transcript guard | Standalone transcript lookup disabled when more than one database selected. |
| Cache clear resets views | `selectedJobId`, `selectedHit`, `transcriptResult`, `spatialResult` all reset on cache clear. |
| Transcript mode UX | Jobs card hidden when `queryMode === 'transcript'`. |
| Hit-scoped DB resolution | `currentDB`, transcript, and spatial lookups resolve database from `selectedHit?.database`. |
| Results table scroll | `scroll={{ x: 'max-content' }}` — horizontal scroll for all columns. |
| Binary index (mmap) | Zero‑decode GFF3 index: xxh3 hash tables + sorted spatial arrays, memory‑mapped. Auto‑detected alongside JSON. |
| `helixblast-prepare` + `verify` | Standalone CLI tools for binary index build and equivalence verification. |
| **IndexedDB‑first architecture** | Job list and results live entirely in browser IndexedDB. Submit → save meta → SSE auto‑opens → terminal → saveFull(idb) → server clears result. No polling, no server‑side job list queries. SSE fallback checks IndexedDB before any network request. Each device isolated. |
| Server: `SetResult` before `SetStatus` | Eliminated race where SSE delivered `{status:"success", result:null}`. |
| Server: Cancel TOCTOU + SetCancel race | Cancel re‑reads status before overwriting; worker checks `IsCancelling()` immediately after `SetCancel`. |
| Server: template → task conversion | `api.go` converts `template` → `advanced_params.task` if `task` not already set. |
| Docker: Debian runtime | Switched to `debian:bookworm-slim` + NCBI BLAST+ tarball. Builder: `golang:1.26.3-trixie`. |
| `split_fasta_v2.sh` | Streams FASTA into `gzip` + `gzip -t` verification. |
| `GET /api/v1/jobs` removed | Endpoint exposed all running jobs to anyone. Frontend no longer uses it. `GET /jobs/{id}` still works (requires knowing the ID). |
| Param whitelist semantics | Whitelist = every parameter scraped from BLAST+ `-help` output, minus server-reserved flags (`query`, `db`, `outfmt`, `num_threads`, `out`) which could otherwise override server-injected args via BLAST+ last-wins semantics. Unknown params still rejected with 400 at submission. |
| Registry TTL | Terminal-state jobs are pruned from the in-memory registry after `result_ttl_hours` (same cadence as storage janitor). Expired job IDs return 404. Bounds memory growth and keeps queue-position rescans proportional to recent jobs. Documented under API → Lifecycle and retention. |
| Data-race fixes | New plain-data `JobSnapshot` type replaces lock-embedding `Job` value copies in JSON responses; `QueuePos` writes now go through locked `SetQueuePos`; concurrent-access regression tests added (`go test -race` clean). |
| SPA deep-link fallback | Static catch-all now serves `index.html` for extensionless paths (hard refresh / deep links to client routes no longer 404) while asset paths keep genuine 404s. Contract documented in architecture → Serving & SPA fallback; unknown client paths render home via a wildcard route. |

