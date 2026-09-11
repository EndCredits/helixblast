# HelixBLAST

Light, modern BLAST web service — single binary, zero external dependencies.

- **BLAST Search**: Web UI for NCBI BLAST+, with job queue, SSE streaming, alignment viewer
- **Multi-Database BLAST**: Search against multiple databases in one submission. Worker runs each DB sequentially, merges and sorts results, tags every hit with its source database. Partial errors per database are collected and displayed.
- **Transcript Lookup**: GFF3-based gene/transcript/CDS coordinate resolution and sequence extraction, backed by local FASTA or Cloudflare Worker + R2. Lives on its own `/transcript` page with a single-database selector (the `/api/v1/transcripts` endpoint always requires a single `db`).
- **Spatial Search**: Chromosome interval lookup. Click a BLAST hit to see overlapping genes and flanking features.
- **Binary Index (mmap)**: GFF3 annotation stored as a memory‑mapped binary with xxh3 hash tables — the **recommended runtime format**, built directly from GFF3 + FASTA with `helixblast-index`. Zero‑decode startup — RSS starts near zero, grows only as query‑touched pages are paged in. JSON indexes remain supported (decoded once, cached by path) for debugging and legacy pipelines; a `.bin` is auto‑detected alongside any configured JSON path.
- **Offline cache**: BLAST results persist in browser IndexedDB (24h TTL). The job list and all results live exclusively in IndexedDB — the server holds no results after SSE delivery. Each browser is an isolated workspace (no cross-device job sharing).

## Quickstart

```bash
# Build server + index tools
make build

# Build the runtime index directly from GFF3 + FASTA (recommended)
./helixblast-index --gff3 annotations.gff3 --fasta genome.fa --out refseq.index.bin
# (legacy: ./helixblast-prepare --json refseq.index.json.gz --out refseq.index.bin)

# Run server — point databases.yaml transcript.index_path at the .bin
./helixblast --config config.yaml
```

## CLI Tools

| Binary | Source | Purpose |
|--------|--------|---------|
| `helixblast` | `cmd/server` | Full BLAST web server (REST + SSE + embedded frontend) |
| `helixblast-index` | `cmd/indexer` | GFF3 + genome FASTA → **binary index directly** (recommended; `--json` optionally keeps a debug intermediate; replaces Node.js `prepare.js`) |
| `helixblast-prepare` | `cmd/prepare` | Legacy/convenience converter: existing JSON → binary, ~3 MB, no BLAST deps |
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
| Registry TTL | Terminal-state jobs are pruned from the in-memory registry after `jobs.result_ttl_hours` (10-minute cadence). Expired job IDs return 404. Bounds memory growth and keeps queue-position rescans proportional to recent jobs. Documented under API → Lifecycle and retention. |
| Data-race fixes | New plain-data `JobSnapshot` type replaces lock-embedding `Job` value copies in JSON responses; `QueuePos` writes now go through locked `SetQueuePos`; concurrent-access regression tests added (`go test -race` clean). |
| SPA deep-link fallback | Static catch-all now serves `index.html` for extensionless paths (hard refresh / deep links to client routes no longer 404) while asset paths keep genuine 404s. Contract documented in architecture → Serving & SPA fallback; unknown client paths render home via a wildcard route. |
| Split BLAST / Transcript pages | `HomePage` split into routable `/blast` and `/transcript` under a shared layout route (header + degraded banner). Cross-page flow via URL params: `?job=` restores the selected BLAST job from IndexedDB on back-navigation; `/transcript?db=&id=` auto-runs lookups and is bookmarkable. BLAST hit buttons navigate instead of mutating local mode state. |
| Spatial search placement (fixup) | Spatial search reverted to its original role: an auxiliary view of BLAST results. It auto-appears below the alignment when a hit's database has `is_chromosome_db: true` (HSP midpoint → overlapping features + flanking genes), and is gated off for non-chromosome databases. Removed from `/transcript`; the redundant "lookup region" button on the alignment was dropped; feature IDs link into `/transcript?db=&id=`. |
| UI redesign (Bio Minimal) | Ant Design v6 token-driven theme in `web/src/theme.ts`: single teal brand seed, slate neutrals, flat-first surfaces (no heavy shadows), 8/12px radius. IGV-style ACGT coloring for alignments and region FASTA via `SequenceText` (nucleotide-only, run-merged, length-capped). Emoji replaced by an inline SVG helix mark (header + favicon). Design rationale documented in architecture → Design system. |
| Dark mode | `system \| light \| dark` preference (default system) via `ThemeModeProvider`, persisted in `localStorage`, resolved before paint by an inline `index.html` script (no FOUC) with live `prefers-color-scheme` tracking. Neutrals flip through `theme.useToken()`; brand + IGV nucleotide palettes have dedicated dark variants. Toggle in the header (sun/moon) and Settings → Appearance. |
| JSON index caching | Decoded JSON readers cached by path (`mtime`+`size` invalidation, per-path load mutex). Measured on a 200K-entry index: per-request JSON load 579 ms / 302 MB churn → cache hit 4.5 µs; end-to-end spatial request 554 ms → 0.5 ms. Binary readers deliberately uncached (open ≈ 25–50 µs, mmap lifecycle per request). Baseline benchmarks: `internal/transcript/bench_test.go`. |
| Spatial range search | `/api/v1/spatial` now takes `start`+`end` (or legacy `pos` as a point); the frontend sends the hit's full subject span (min/max across HSPs, minus-strand normalized). Every returned feature keeps its full indexed span — never clipped to the query window. Upstream flank fixed to nearest-by-`End` (was last-in-start-order, which nesting made wrong). `IndexReader.Spatial(chr)` whole-chromosome copy replaced by `SpatialSearch(chr,start,end)`: binary search + one bounded backward pass — O(log n + window), ~8 µs / 736 B / 11 allocs on a 1.5 M-feature chromosome (was 84 ms / 132 MB). Production readers cross-checked against a naive reference; dead v1 `SpatialLookup` removed. |
| Server-side storage layer removed | The entire orphaned `internal/storage` + `internal/janitor` subsystem (local + S3/MinIO backends, presigned URLs, TTL janitor) was deleted — it had zero live callers after the IndexedDB-first architecture stopped persisting results. Genome/index data is served via local filesystem or Cloudflare Worker + R2 (S3 random reads were unoptimizable). `config.yaml`'s `storage`/`s3` sections are gone; the surviving TTL knob moved to `jobs.result_ttl_hours` (in-memory registry only). `/health` drops `storage_backend`; the header status line shows version + workers. |
| Whitelist fail-fast | If no BLAST+ `-help` output can be parsed at startup, the server now exits with `Fatalf` instead of booting with a nil whitelist that allowed every parameter — fail-open silently disabled the reserved-flag protection the whitelist exists to provide. All `whitelist != nil` guards removed. |
| Index hardening | `index.Open()` now validates every header-derived section range against the file size (overflow-guarded), so truncated/corrupted `.bin` files are rejected at open instead of panicking in hot paths. A byte-flip fuzz test also caught a latent **use-after-unmap**: the bad-magic/unsupported-version error paths formatted `hdr` after `Close()` had already unmapped it — the header is now copied to the heap before any error branch. |
| Shutdown race fixes | `pool.Stop()` sets a `stopped` flag and closes the queue channel under the same mutex `Submit` holds — submitting during the shutdown window now returns `503 Service Unavailable` instead of panicking on a closed channel. The static file handler refuses `.go` paths (`//go:embed *` otherwise serves its own source file). |
| Misc | i18next plural keys migrated to v4 `_one`/`_other` (legacy `_plural`/camelCase variants never resolved — count>1 messages were silently falling back); added `.dockerignore` (was absent: `.git`, `node_modules`, build artifacts streamed into every image build). |
| Dead-code sweep | Removed: `Pool.List`/`queuePosUnsafe`/`doneCh` (leftovers of the deleted `GET /jobs`), `ExecConfig.Timeout` (set but never read), `builder.align` (identity function), 11 unused `theme.ts` exports, 21 unused i18n keys (en+zh). Alignment gutter labels stay as literal `Query`/`Sbjct` — standard BLAST notation, not UI copy. |
| Submit dedupe & fallback docs | `POST /jobs` with `dbs: ["nt","nr","nt"]` now dedupes (same DB twice ran BLAST twice and double-counted hits). transcript-lookup.md documents the exact Worker-fallback boundary: Worker is consulted only when no local FASTA source is configured — a missing chromosome file in a configured `fasta_dir` fails loudly, it does not spill to the Worker. |
| Job ID entropy | `hxb-` job IDs are now 128-bit (16 random bytes, 32 hex chars) instead of 32-bit. The ID is the sole capability token on unauthenticated job endpoints: 32 bits was brute-forceable against results left undelivered (API consumers that never poll keep their result until TTL prune) and made silent registry collisions (map overwrite → cross-user result bleed) a live if rare risk. `megablast` also removed from the valid *programs* list — modern BLAST+ has no such binary; it is a `task` of `blastn`, which the UI and API already use correctly. |

