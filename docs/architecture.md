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
| Lookup | Go `map[string]T` O(1) | FNV‑1a hash table, linear probing, O(1) |
| Load | Full `json.Decode` → all data in RAM | `mmap` → OS pages in on demand |
| Startup RSS | 60–80 MB | ~0 (virtual address space only) |
| Steady RSS | ~40 MB (all maps resident) | 5–15 MB (only accessed pages) |
| File size | 15–30 MB (gzipped) | 22–35 MB (uncompressed) |
| Build tool | `prepare.js` (Node.js) | `helixblast-prepare` (Go, ~3 MB) |

### `LoadIndexAuto` auto‑detection

```
databases.yaml: transcript.index_path = refseq.index.json.gz

   1. Check refseq.index.bin exists  →  mmap + hash table
   2. Fallback                         →  json.Decode full load
```

No config change required. Place `.bin` alongside the existing `.json.gz` and restart — the server picks it up.

### `helixblast-prepare`

```bash
go build -o helixblast-prepare ./cmd/prepare
./helixblast-prepare --json refseq.index.json.gz --out refseq.index.bin
```

Reads the GFF3 JSON index (produced by `prepare.js`) and writes an mmap‑friendly binary. The binary is ~same size as gzipped JSON, but loads with zero decode cost.

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

- **Hash function**: FNV‑1a 64‑bit (offset basis `14695981039346656037`, prime `1099511628211`)
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

LookupEntry(id)
  → hashStr(id) → lookupHash (probe slots, verify string)
  → EntryRecord at computed offset
  → Entry{Chr, Start, End, Strand, Type, Gene}

LookupFamily(gene)
  → hashStr(gene) → lookupHash
  → FamilyRecord → DataOffset
  → uint32 string refs → stringAt() for each ID

Spatial(chr)
  → linear scan SpatialHeader list for chr
  → SpatialFeatureRec array at DataOffset

Close()
  → unix.Munmap → f.Close
```

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
