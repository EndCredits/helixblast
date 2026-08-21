# Architecture

## Single binary

HelixBLAST compiles to a **single static binary** (~13MB). The React frontend is embedded at build time via `//go:embed`, served from memory with zero disk I/O. No Nginx, no CDN, no external assets.

## Why Go

- **os/exec**: Native process management for BLAST+ — process isolation, signal handling, stdout/stderr capture
- **bufio**: Streaming BLAST output parsing without loading results into memory
- **chi/v5**: Zero-allocation radix tree router, <2MB memory footprint
- **Goroutines**: Lightweight concurrency for worker pool — fixed-size goroutine pool with channel-based job queue
- **Single binary**: Cross-compilation to any target (linux/amd64, darwin/arm64, etc.)

## Why no database

Job state lives in memory (Go maps + channels). No Redis, no PostgreSQL, no SQLite. This is intentional:

- **Anonymous access**: No user accounts to persist
- **Ephemeral data**: Results auto-deleted after 24h — no long-term storage needed
- **Single-server**: No distributed coordination required

The trade-off: server restart loses job history. Acceptable for the use case (ad-hoc analysis, no archival requirements).

## Index: Binary vs JSON

HelixBLAST supports two GFF3 index formats, auto-detected at runtime via `transcript.LoadIndexAuto()`:

| | JSON (`.json.gz`) | Binary (`.bin`) |
|---|---|---|
| Lookup | Go `map[string]T` O(1) | xxh3 hash table, linear probing, O(1) |
| Load | Full `json.Decode` → all data in RAM | `mmap` → OS pages in on demand |
| Startup RSS | 60–80 MB | ~0 (virtual address space only) |
| Steady RSS | ~40 MB (all maps resident) | 5–15 MB (only accessed pages) |
| File size | 15–30 MB (gzipped) | 22–35 MB (uncompressed) |
| Build tool | `helixblast-index` (Go, GFF3+FASTA → index) | `helixblast-prepare` (Go, JSON → bin) |

### `LoadIndexAuto` auto‑detection

```
databases.yaml: transcript.index_path = refseq.index.json.gz

   1. Check refseq.index.bin exists  →  mmap + hash table
   2. Fallback                         →  json.Decode full load
```

No config change required. Place `.bin` alongside the existing `.json.gz` and restart — the server picks it up.

### `helixblast-index` (GFF3 → index)

```bash
make build-index
./helixblast-index --gff3 annotations.gff3 --fasta genome.fa --out refseq.index.bin
# optional: --json refseq.index.json.gz keeps an intermediate JSON for debugging/verify
```

Parses GFF3 + genome FASTA (replacing the former Node.js `prepare.js`) and writes a binary index directly. Semantics match the old pipeline: Parent-chain coordinate resolution, gene families, per-transcript exon/CDS coords, spatial features, and FASTA byte offsets.

### `helixblast-prepare` (JSON → bin)

```bash
go build -o helixblast-prepare ./cmd/prepare
./helixblast-prepare --json refseq.index.json.gz --out refseq.index.bin
```

Reads a GFF3 JSON index (e.g. produced by `helixblast-index --json`) and writes an mmap‑friendly binary. The binary is uncompressed (~same size as decompressed JSON), but loads with zero decode cost.

### `verify`

```bash
go run ./cmd/verify --json refseq.index.json.gz
# → VERIFIED: JSON and binary indices produce identical results.
```

Builds a temporary `.bin` from JSON, then compares every entry, family, coord, and Fasta‑index field. Reports exact mismatches.

## Binary Index Format

### Layout

| Offset | Content | Size |
|--------|---------|------|
| 0 | `Header` — magic `HXBI`, version=1, entry/family/coord/spatial counts, section offets, string pool off/size | 88 B |
| `hdr.EntriesOffset` | Entry hash table: `{hash uint64, val uint64}` × `nextPow2(entryCount×2)` | slotCount×16 B |
| … after hash | Entry records: `{chr_off, start, end, strand_off, type_off, gene_off}` × entryCount | entryCount×24 B |
| `hdr.FamiliesOffset` | Family hash table | slotCount×16 B |
| … after hash | Family records: `{tx_count, cds_count, exon_count, _, data_off}` × familyCount | familyCount×24 B |
| … after records | Family string‑ref data: `uint32` arrays (transcript IDs, CDS IDs, exon IDs) | variable |
| `hdr.CoordsOffset` | Coord hash table + records + exon/CDS `{start, end}` pair arrays | variable |
| `hdr.SpatialOffset` | 4‑byte chr count + `SpatialHeader{chr_off, feat_count, data_off}` list + per‑chr `SpatialFeatureRec{start, end, id_off, type_off}` arrays | variable |
| `hdr.FastaIdxOffset` | 4‑byte chr count + `FastaIndexEntry{chr_off, _, offset}` list | 4 + chrCount×16 B |
| `hdr.StringPoolOff` | Null‑terminated string pool — all label strings interned here | poolSize |

### Hash table design

- **Hash function**: xxh3 64‑bit (via `github.com/zeebo/xxh3`); hash value 0 is remapped to 1 because `hash == 0` is the empty-slot sentinel
- **Collision resolution**: Open addressing with linear probing
- **Load factor**: ~50% (capacity = `nextPow2(count × 2)`)
- **Empty slot**: `hash == 0`
- **Slot value**: high 32 bits = array index, low 32 bits = string pool offset for collision verification

### Struct alignment

All structs include explicit Go alignment padding fields (e.g. `_ uint32` before `uint64`). This ensures `binary.Write` and `unsafe.Pointer` casts use the same byte layout. `binary.Write` outputs packed field‑by‑field; the padding fields carry zero bytes to maintain 8‑byte alignment for subsequent `uint64` fields.

The format is designed for in‑process use via `mmap` + `unsafe` pointer casts — **not** for cross‑language interchange.

### Reader internals

```
Open(path)
  → os.Stat → unix.Mmap(PROT_READ, MAP_SHARED)
  → Header (*Header)(unsafe.Pointer(&data[0]))
  → verify magic + version
```

The binary reader implements the `transcript.IndexReader` interface, which bridges JSON and binary backends:

| Method | Returns | Used by |
|--------|---------|---------|
| `LookupEntry(id)` | `(*Entry, bool)` | `LookupWithIndex` |
| `LookupFamily(gene)` | `(*Family, bool)` | `LookupWithIndex` (gene family) |
| `LookupCoords(id)` | `(*CoordRegions, bool)` | `LookupWithIndex` (exon/CDS coords) |
| `Spatial(chr)` | `([]SpatialFeat, error)` | `SpatialLookupV2` |
| `FastaOffset(chr)` | `(int64, bool)` | `extractSequence` |
| `FastaIndexMap()` | `map[string]int64` | `extractSequence` (bulk) |
| `Close()` | `error` | Resource cleanup |

`LoadIndexAuto` returns either a `*index.Reader` (binary) or a `*jsonIndexReader` (JSON), both implementing this interface. `LookupWithIndex` and `SpatialLookupV2` accept `IndexReader`, making them format‑agnostic.

## Concurrency model

```
┌─ HTTP Server ─────────────┐
│  chi router                │
│  POST /jobs ───────┼──→ jobCh (buffered channel)
│  GET /jobs/:id/events ┼──→ job.Subscribe() → SSE stream
│  DELETE /job ────────┼──→ job.Cancel()
└───────────────────────┘
        │
┌─ Worker Pool ──────────────────────────────────┐
│  [worker 1] ← jobCh  → BLAST exec              │
│  [worker 2] ← jobCh  → BLAST exec              │
│  [worker N] ← jobCh  → BLAST exec              │
│                                                  │
│  Pool size = min(max_jobs, CPU/mem_adaptive)    │
│  Queue capacity = max_jobs                      │
│  429 when queue full                            │
└─────────────────────────────────────────────────┘
```

### Resource adaptation

At startup, the system probes CPU cores and available RAM to compute a safe concurrency level:

- `cpu_limit = max(1, NumCPU / cpu_per_job)`
- `memory_limit = <2GB→2, <4GB→5, default 20` (macOS/non‑Linux: always 4)
- `actual = min(max_jobs, cpu_limit, memory_limit)`

This prevents OOM on low-resource machines. The `/health` endpoint reflects the degraded state.

### Multi‑database BLAST

When `POST /api/v1/jobs` receives `dbs: ["nr", "nt", "refseq"]`, a single Job is created. The worker loops over `job.Databases` sequentially:

```
for _, dbName := range job.Databases {
    job.SetProgress("BLAST against " + dbName + " ...")
    hits, err := p.execFn(ctx, job, dbName)
    if err != nil {
        errs = append(errs, {dbName, err.Error()})
        continue  // don't block remaining databases
    }
    tag each hit with dbName
    collect hits
}
```

Results are merged, sorted by `total_score` descending, and trimmed to top 200. Partial errors are included in `BlastResult.Errors` and displayed at the top of the results view.

## Job lifecycle

```
Submit → Pending → Queued → Running → Success / Failed
                            ↘ Cancelled (soft cancel)
```

**Soft cancel**: Does NOT kill the BLAST process (no SIGKILL). Sets an `atomic.Bool` flag. When BLAST completes, the worker checks the flag — if set, discards the result and marks the job `cancelled`. This avoids orphaned temp files and zombie processes.

**Timeout**: Each job has a 2-hour `context.WithTimeout`. BLAST receives the context and is killed by the OS when it expires.

**Cleanup**: A janitor goroutine runs every 10 minutes, scanning for jobs older than `result_ttl_hours`. Both local files and S3 objects are cleaned. The worker pool runs a second goroutine on the same cadence that prunes terminal-state jobs from the in-memory registry once they pass `result_ttl_hours`, bounding registry memory to ~24h of job history and keeping queue-position rescans O(recent jobs) instead of O(all-time jobs).

## SSE streaming

Job detail updates use Server-Sent Events with a subscriber pattern:

```
Job.SetStatus() → job.notify() → push to all subscriber channels
                                    │
SSE handler ← subCh ←── Subscribe() ─┘
```

Each SSE connection creates a subscriber channel on the Job. Status changes push a message to all subscribers. When the job reaches a terminal state, the SSE handler calls `ClearResult()` and returns. The frontend opens SSE automatically on job creation via `EventSource`, with exponential backoff reconnection (1s→2s→…→30s, max 10 retries). After 10 failures, it switches to IndexedDB polling (5s interval) — it never queries the server directly.

## BLAST parameter whitelist

At startup, HelixBLAST runs `blastn -help`, `blastp -help`, `blastx -help`, `tblastn -help`, `tblastx -help` and parses the output to build a whitelist of valid parameters. User-supplied `advanced_params` are validated against this list at **job submission time** — unknown parameters are rejected with `400 Bad Request` and the job never enters the queue. This prevents arbitrary flag injection.

Default BLAST parameters when not overridden: `-max_target_seqs 5000`, `-evalue 10`. The parser further limits results to 20 hits per database (merged to top 200 across databases).

## Rate limiting

Token bucket algorithm: 100 requests/second per IP, burst capacity 120. Returns `429 Too Many Requests` with `Retry-After: 5`. Inactive IPs are cleaned from the map every 5 minutes.

## Shutdown

Graceful shutdown on SIGINT/SIGTERM:

1. Cancel all running/queued jobs
2. Wait for workers to finish current work (30s timeout)
3. HTTP server graceful shutdown (5s timeout)
4. Exit

## Frontend

### Serving & SPA fallback

Client-side routes (react-router `BrowserRouter`): `/`, `/settings`, wildcard → home. Server side, API routes (`/api/v1/*`, `/health`) are registered **before** the static catch-all and never reach it.

The catch-all (`serveFrontend` in `internal/api`) implements a self-maintaining fallback — deliberately **no hand-written exclusion list**:

| Request | Behavior |
|---|---|
| Path maps to a real embedded file | Served verbatim (JS/CSS bundles, favicon) |
| Extensionless path with no backing file | Rewritten to `/` → serves `index.html`; react-router takes over |
| Path carries a file extension but has no backing file | Genuine `404` |

Why this shape:

- `/api/*` and `/health` are excluded by route precedence, not by a list — new API endpoints inherit this automatically.
- Any extensionless URL falls back to the app shell, so adding a client route needs zero server changes.
- Broken asset URLs intentionally return 404 instead of HTML: they fail loudly in devtools instead of silently parsing HTML as JavaScript.
- **Corollary / the one rule to keep**: client-side routes must never contain a file extension.

### Tech stack

| Component | Library | Why |
|-----------|---------|-----|
| Framework | React 18 | Declarative UI, mature ecosystem |
| Build | Vite 6 | ESM-native, fast HMR, tree-shaking |
| UI | Ant Design 5 | Mature component library, form/table/tag/card |
| Data fetching | @tanstack/react-query | Cache dedup, background GC, SSE + IndexedDB for jobs |
| State push | EventSource (SSE) | Server-push for job status, auto-reconnect |
| Client storage | IndexedDB | Browser-local persistence (24h TTL), survives restarts |

### IndexedDB‑first data flow

The browser's IndexedDB is the **sole source of truth** for all job state. The server holds no results after SSE delivery.

```
Submit POST /jobs
  → res.job_id
  → saveJobMeta(idb)           ← save {job_id, status:"queued", ...}
  → reloadLocalJobs(idb)       ← refresh UI list
  → setSelectedJobId
  → loadCachedJob(idb)          ← guard: if terminal+result, skip SSE
  → EventSource(/jobs/{id}/events)  ← SSE auto-opens

SSE stream
  → queued → running → success
  → saveJobFull(idb)           ← overwrite with full result
  → onTerminal → reloadLocalJobs(idb)
  → server: ClearResult()      ← free memory

Page reload
  → loadJobs(idb)
  → mergedJobs = savedJobs     ← no server fetch
  → click job → loadCachedJob(idb) → guard → SSE or skip
```

**No polling.** The job list is built entirely from IndexedDB. SSE delivers real-time status for the selected job. The `GET /api/v1/jobs` endpoint has been removed — it no longer exists.

**No cross-device sharing.** Each browser's IndexedDB is isolated. A job submitted on device A is invisible on device B. The `api.fetchJob(jobId)` fallback has been removed — if a job isn't in local IndexedDB, it doesn't exist for that client.

### SSE resume and fallback

```
SSE onerror
  → check react‑query cache for terminal → exit
  → retry < 10: exponential backoff (1s → 2s → ... → 30s)
  → retry > 10: switch to IndexedDB polling
                 → loadCachedJob(idb) every 5s
                 → if terminal+result: stop, saveFull, reloadList
                 → never queries server
```

### Server‑side result lifecycle

```
worker: SetResult → SetStatus(Success) → notify()
SSE:    send(snap) → Flush → snap.Status terminal? → ClearResult()
        ClearResult sets job.Result = nil — memory freed immediately
```

`SetResult` is called **before** `SetStatus(Success)` to ensure the SSE snapshot includes the result. `ClearResult()` is called after the SSE handler confirms delivery (either on initial `send()` or on subscriber notification). The `GET /api/v1/jobs/{id}` handler also calls `ClearResult()` after serving the snapshot, for API consumers.

### Job list architecture (removed)

The frontend previously merged a server‑side job list (`GET /api/v1/jobs`) with IndexedDB cache and ran a 3s polling loop for active jobs. This has been removed in favor of the IndexedDB‑only model above.

### Spatial search

When a BLAST database uses chromosome sequences (`is_chromosome_db: true`), clicking a hit triggers `/api/v1/spatial` which queries a pre-built interval index in the GFF3 data. The index maps each chromosome to a sorted array of features (gene, mRNA, CDS, exon). Overlapping features and the nearest upstream/downstream genes are resolved in a single linear scan. Clicking any gene/transcript/CDS ID jumps to Transcript Lookup to view the full sequence.

### State management

The app uses local React state (`useState`) + react‑query cache for server data (health, databases, job detail). No Redux, no Zustand. The job list is managed via `useState(savedJobs)` backed by IndexedDB — not react‑query.
