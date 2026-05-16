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

## Concurrency model

```
┌─ HTTP Server ─┐
│  chi router    │
│  POST /jobs ───┼──→ jobCh (buffered channel)
│  GET /jobs   ←─┼──← pool.jobs (sync.RWMutex map)
│  DELETE /job ←─┼──→ job.Cancel() → context cancel
└────────────────┘
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
- `memory_limit = <2GB→2, <4GB→5, default 20`
- `actual = min(max_jobs, cpu_limit, memory_limit)`

This prevents OOM on low-resource machines. The `/health` endpoint reflects the degraded state.

## Job lifecycle

```
Submit → Pending → Queued → Running → Success / Failed
                            ↘ Cancelled (soft cancel)
```

**Soft cancel**: Does NOT kill the BLAST process (no SIGKILL). Sets an `atomic.Bool` flag. When BLAST completes, the worker checks the flag — if set, discards the result and marks the job `cancelled`. This avoids orphaned temp files and zombie processes.

**Timeout**: Each job has a 2-hour `context.WithTimeout`. BLAST receives the context and is killed by the OS when it expires.

**Cleanup**: A janitor goroutine runs every 10 minutes, scanning for jobs older than `result_ttl_hours`. Both local files and S3 objects are cleaned.

## SSE streaming

Job detail updates use Server-Sent Events with a subscriber pattern:

```
Job.SetStatus() → job.notify() → push to all subscriber channels
                                    │
SSE handler ← subCh ←── Subscribe() ─┘
```

Each SSE connection creates a subscriber channel on the Job. Status changes push a message to all subscribers. When the job reaches a terminal state or the client disconnects, the channel is cleaned up.

This replaces timer-based polling: the server pushes only when state changes, not on a fixed interval. The frontend uses `EventSource` with exponential backoff reconnection and a 5s HTTP polling fallback after 10 failures.

## BLAST parameter whitelist

At startup, HelixBLAST runs `blastn -help`, `blastp -help`, etc. and parses the output to build a whitelist of valid parameters. User-supplied `advanced_params` are validated against this list — unknown parameters are rejected with `400`. This prevents arbitrary flag injection.

## Rate limiting

Token bucket algorithm: 100 requests/second per IP, burst capacity 120. Returns `429 Too Many Requests` with `Retry-After: 5`. Inactive IPs are cleaned from the map every 5 minutes.

## Shutdown

Graceful shutdown on SIGINT/SIGTERM:

1. Cancel all running/queued jobs
2. Wait for workers to finish current work (30s timeout)
3. HTTP server graceful shutdown (5s timeout)
4. Exit

## Frontend

### Tech stack

| Component | Library | Why |
|-----------|---------|-----|
| Framework | React 18 | Declarative UI, mature ecosystem |
| Build | Vite 6 | ESM-native, fast HMR, tree-shaking |
| UI | Ant Design 5 | Mature component library, form/table/tag/card |
| Data fetching | @tanstack/react-query | Auto-polling, cache dedup, background GC |
| State push | EventSource (SSE) | Server-push for job status, auto-reconnect |
| Client storage | IndexedDB | Browser-local persistence (24h TTL), survives restarts |

### Client-side persistence

BLAST results are stored in the browser's IndexedDB (database `helixblast`, store `cache`). The server holds results only until the client acknowledges receipt via SSE or polling. At that point, `ClearResult()` is called on the server-side job to free memory. The client-side entry includes a `created_at` timestamp — entries older than 24 hours are automatically purged on next page load.

This means:
- Server memory is only consumed by active (queued/running) jobs
- Refreshing the page or restarting the server does not lose previously viewed results
- Each browser sees only its own IndexedDB entries — no user tracking or server-side persistence

### Spatial search

When a BLAST database uses chromosome sequences (`is_chromosome_db: true`), clicking a hit triggers `/api/v1/spatial` which queries a pre-built interval index in the GFF3 data. The index maps each chromosome to a sorted array of features (gene, mRNA, CDS, exon). Overlapping features and the nearest upstream/downstream genes are resolved in a single linear scan. Clicking any gene/transcript/CDS ID jumps to Transcript Lookup to view the full sequence.

### States managed without a state library

The app uses local state (`useState`) + react-query cache. No Redux, no Zustand. Query keys: `['health']`, `['databases']`, `['jobs']`, `['job', id]`.

### Lazy polling

`useJobs()` only polls `GET /api/v1/jobs` when there's at least one non-terminal job. When all jobs are complete, polling stops. The `refetchInterval` is a function of query state, not a constant.
