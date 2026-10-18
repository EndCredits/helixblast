# Architecture

## Runtime shape

HelixBLAST is one Go HTTP server with an embedded React single-page application. The build copies the frontend bundle into `embed/`, and `//go:embed` puts it in the binary. The server reads no frontend files from disk at runtime.

The host must provide NCBI BLAST+. The server starts BLAST+ as a child process and does not bundle the BLAST+ executables or databases.

The app shell and JavaScript bundle are self-contained. The UI loads Google Sans Flex, Noto Sans SC, and Fira Code from `fonts.googleapis.com`, so a host without internet access falls back to the system font stack.

## Why Go

- `os/exec` and `context` start BLAST+, capture stdout and stderr, and terminate the process on cancel or timeout.
- `bufio` parses tabular BLAST output. The full stdout is buffered before parsing. `-max_target_seqs 5000` limits subject sequences; the 20-hit cap is applied after parsing.
- `chi` provides the route tree for the API and the static fallback.
- A fixed set of goroutines consumes jobs from a buffered channel.
- The server cross-compiles to Linux and macOS targets without CGO.

## Why there is no database

Job state lives in Go maps and channels. There is no Redis, PostgreSQL, or SQLite.

- There are no user accounts, so there is no durable ownership model.
- Results are ephemeral. The browser's IndexedDB is the durable copy.
- A single server does not need distributed coordination.

The trade-off is that a restart loses every job record and every result that has not reached a browser. That fits ad-hoc analysis, not archival use.

## Index formats

HelixBLAST reads two GFF3 index formats. `transcript.LoadIndexAuto` chooses between them at runtime.

| | JSON (`.json.gz`) | Binary (`.bin`) |
|---|---|---|
| Lookup | Go map, O(1) | xxh3 hash table, O(1) |
| Load | Full `json.Decode` on the first request, then cached by path | `mmap` per request, pages in on demand |
| Cache | Yes, invalidated by modification time and size. `Close` is a no-op, so a shared reader stays valid. | No. Opening costs tens of microseconds, and each reader owns an mmap that must be closed. |
| Resident memory | About 0.4 KB of live heap per entry, or 2 to 3 times the equivalent `.bin` file size. One cached copy per configured JSON index. | No Go heap cost between requests. During a request, the resident set is limited to the pages the query touches and is released on close. |
| File size | 15 to 30 MB gzipped in the calibration index | 22 to 35 MB uncompressed in the calibration index |
| Build tool | `helixblast-index --json` for an intermediate or debug copy | `helixblast-index` directly from GFF3 and FASTA |

The numbers come from a synthetic 200K-entry index on an Apple M1. They scale with the annotation, so the useful invariants are ratios rather than absolute sizes:

- JSON resident cost is about 0.4 KB per entry, or 2 to 3 times the binary file size.
- An uncached JSON load allocates about 4 times the resident size per request. On the calibration index, that was 579 ms and 302 MB of allocation churn.
- A JSON cache hit is O(1) in index size: one `stat` and a map lookup. The same calibration measured 4.5 microseconds.
- A binary index adds no decoded full-index heap. Its memory is page-granular and self-releasing.

Use a `.bin` built directly from GFF3 and FASTA unless you need JSON for inspection, for the `verify` tool, or for a legacy pipeline that only has JSON left.

The binary format is version 2. Version 1 indexes used FNV-1a and must be rebuilt.

## `LoadIndexAuto`

```text
path ends in .bin
  -> index.Open(path)

path is JSON (for example refseq.index.json.gz)
  -> if refseq.index.bin exists, index.Open(refseq.index.bin)
  -> otherwise, loadJSONCached(path)
```

The JSON cache is a process-global map keyed by path. Each entry has its own mutex, stores the file's modification time and size, and reloads when either changes. The cache has no eviction. `jsonIndexReader.Close` is a no-op, so callers can close a shared reader safely.

Binary readers are deliberately not cached. Opening one is cheap, and each reader must own and release its own mmap.

Both readers implement `transcript.IndexReader` and duplicate the 8 MB `spatialBackScan` constant. Keep the two constants in sync.

## Build indexes

### `helixblast-index`

```bash
make build-index
./helixblast-index --gff3 annotations.gff3 --fasta genome.fa --out refseq.index.bin
# Optional intermediate JSON:
./helixblast-index --gff3 annotations.gff3 --fasta genome.fa \
  --out refseq.index.bin --json refseq.index.json.gz
```

This tool replaces the former Node.js `prepare.js`. It parses GFF3 and genome FASTA and writes the binary index directly. The index contains:

- `entries`: every GFF3 ID whose Parent chain resolves to a gene, mRNA, or transcript, plus every `Name` alias. Gene, mRNA, and transcript rows use their own coordinates; child features use the nearest annotated ancestor.
- `families`: gene to transcript, CDS, and exon ID lists.
- `coords`: exon and CDS coordinate arrays per transcript.
- `fasta_index`: byte offset of each chromosome header in the genome FASTA.
- `spatial`: per-chromosome features sorted by start. The index stores gene, mRNA, CDS, and exon records; the spatial API filters the result to genes.

Point `transcript.index_path` at the `.bin`. The JSON file is only an intermediate.

### `helixblast-prepare`

```bash
make build-prepare
./helixblast-prepare --json refseq.index.json.gz --out refseq.index.bin
```

This converts an existing JSON index when the GFF3 source is gone. Prefer `helixblast-index` when the sources are available. The binary is uncompressed, roughly the size of the decoded JSON, and loads without a decode step.

### `verify`

```bash
make build-verify
./verify --json refseq.index.json.gz
# VERIFIED: JSON and binary indices produce identical results.
```

`verify` builds a temporary `.bin` from the JSON index and compares every entry, family, coordinate array, and FASTA offset. It does not compare spatial records.

## Binary index format

### Layout

| Offset | Content | Size |
|---|---|---|
| 0 | Header: magic `HXBI`, version 2, section counts, section offsets, string pool offset and size | 88 B |
| `EntriesOffset` | Entry hash table: `{hash uint64, value uint64}` per slot, `nextPow2(entryCount * 2)` slots | slots x 16 B |
| after entry hash | Entry records: `{chr_off, start, end, strand_off, type_off, gene_off}` | entries x 24 B |
| `FamiliesOffset` | Family hash table | slots x 16 B |
| after family hash | Family records: `{tx_count, cds_count, exon_count, _, data_off}` | families x 24 B |
| after family records | Family string references: `uint32` arrays for transcript, CDS, and exon IDs | variable |
| `CoordsOffset` | Coord hash table, coord records, then exon and CDS `{start, end}` pairs | variable |
| `SpatialOffset` | 4-byte chromosome count, `SpatialHeader{chr_off, feat_count, data_off}` list, then per-chromosome `SpatialFeatureRec{start, end, id_off, type_off}` arrays | variable |
| `FastaIdxOffset` | 4-byte chromosome count, then `FastaIndexEntry{chr_off, _, offset}` list | 4 + chromosomes x 16 B |
| `StringPoolOff` | Null-terminated string pool shared by all sections | pool size |

### Hash table design

- Hash function: xxh3 64-bit.
- Hash value 0 is remapped to 1 because 0 marks an empty slot.
- Collision resolution: linear probing.
- Load factor: about 50 percent (`nextPow2(count * 2)` slots).
- Empty slot: `hash == 0`.
- Slot value: high 32 bits hold the record index, low 32 bits hold the string pool offset used to verify the key.

### Struct alignment

The structs include explicit padding fields so `binary.Write` and `unsafe.Pointer` casts see the same layout. For example, `FastaIndexEntry` has a `_ uint32` before its `int64` offset, and `FamilyRecord` has a `_ uint32` before its `uint64` data offset. The padding fields carry zero bytes and keep the following 64-bit fields 8-byte aligned.

The format is for in-process use through mmap and unsafe pointer casts. It is not a cross-language interchange format.

`Open` validates every header-derived section range against the file size before any lookup. The arithmetic is overflow-guarded. A truncated or hand-corrupted file therefore fails at open, not in a hot path. The header is copied to the heap before any error branch can call `Close` and unmap the pages. Corruption tests cover truncation, field corruption, and exhaustive header byte flips in `internal/index/reader_corrupt_test.go`.

### Reader internals

```text
Open(path)
  -> os.Stat
  -> unix.Mmap(PROT_READ, MAP_SHARED)
  -> copy Header to the heap
  -> check magic and version
  -> validate section ranges
```

The reader implements `transcript.IndexReader`:

| Method | Returns | Used by |
|---|---|---|
| `LookupEntry(id)` | `(*Entry, bool)` | `LookupWithIndex` |
| `LookupFamily(gene)` | `(*Family, bool)` | `LookupWithIndex` |
| `LookupCoords(id)` | `(*CoordRegions, bool)` | `LookupWithIndex` |
| `SpatialSearch(chr, start, end)` | `(*SpatialHits, error)` | `SpatialLookup` |
| `FastaOffset(chr)` | `(int64, bool)` | `extractSequence` |
| `FastaIndexMap()` | `map[string]int64` | `extractSequence` |
| `Close()` | `error` | Resource cleanup |

`LoadIndexAuto` returns either `*index.Reader` or `*jsonIndexReader`. Both satisfy the interface, so `LookupWithIndex` and `SpatialLookup` do not depend on the storage format.

## Concurrency model

```text
HTTP server
  POST /api/v1/jobs       -> jobCh (buffered channel)
  GET  /jobs/{id}/events  -> job.Subscribe() -> SSE stream
  DELETE /jobs/{id}       -> job.Cancel()

Worker pool
  worker 1 .. worker N <- jobCh -> BLAST+ process
  N = actual concurrency
  queue capacity = blast.max_jobs
```

`Submit` sets the job to `pending`, sends it to the channel, then sets `queued` and records its queue position. If the channel is full, `Submit` returns an error and the API responds with `429`. A submit that arrives after `Stop` begins returns `503`.

### Resource adaptation

At startup the server computes:

```text
cpu_limit    = max(1, NumCPU / cpu_per_job)
memory_limit = available RAM < 2 GB  -> 2
               available RAM < 4 GB  -> 5
               otherwise             -> 20
actual       = min(max_jobs, cpu_limit, memory_limit)
```

Linux reads `MemAvailable` from `/proc/meminfo`. Other operating systems use a fixed memory limit of 4. The worker pool is created with `actual` workers and a queue capacity of `max_jobs`.

The `/health` handler recomputes resources on each request, while the pool size stays fixed at startup. If available memory changes, the reported capacity can differ from the number of running workers.

### Multiple databases

When a job lists several databases, one worker processes them in order:

```text
for each dbName in job.Databases:
    set progress "BLAST against <dbName> ..."
    hits, err := execFn(ctx, job, dbName)
    if err:
        append DatabaseError{dbName, err}
        continue
    tag each hit with dbName
    append hits to the merged list
```

The API removes duplicate database names before it creates the job. After the loop, the worker sorts all hits by `total_score` and keeps the top 200. Each database contributes at most 20 hits.

If some databases fail and at least one succeeds, the result includes `errors` and the job succeeds. If every database fails, the job becomes `failed` and the job `error` field contains the combined message. A cancelled job records a `cancelled` error for each database it did not run.

## Job lifecycle

```text
Submit -> pending -> queued -> running -> success
                                      -> failed
         -> cancelled
```

A running job gets a 2-hour `context.WithTimeout`. `exec.CommandContext` passes that context to BLAST+, so the process is killed when the context expires or is cancelled.

Cancel is not a soft flag. `Job.Cancel` sets the cancellation flag and invokes the context cancel function. A queued job moves to `cancelled` immediately. A running BLAST process is terminated, and the worker marks the job `cancelled` after it observes the flag. The worker still records a cancellation error for databases it did not reach.

The worker sets the result before the terminal status. It clears `progress` on success or cancellation.

The registry pruner runs every 10 minutes. It removes terminal jobs whose last update is older than `jobs.result_ttl_hours`. Running jobs are never pruned. The result payload itself is cleared earlier, when the first terminal snapshot is delivered.

A job ID is the only credential on the job endpoints. IDs are 128-bit random values with the `hxb-` prefix.

## SSE streaming

Each SSE connection subscribes to a job:

```text
Job.SetStatus / SetProgress / SetResult
  -> job.notify()
  -> every subscriber channel receives a wake-up
  -> SSE handler reads a fresh JobSnapshot, marshals it, writes "data: ..."
```

The server sends a snapshot on every change, including result publication. The worker calls `SetResult` before `SetStatus(Success)`, so the terminal snapshot contains the result.

The stream ends on `success`, `failed`, or `cancelled`. The handler calls `ClearResult` after it sends the terminal snapshot. `GET /api/v1/jobs/{id}` does the same after a terminal response. The payload is not retained for a later reader.

The frontend uses `EventSource`. Transient connection drops are left to the browser's native reconnect. When the stream reaches `CLOSED`, the client retries with exponential backoff starting at 2 seconds and capped at 30 seconds. After 10 failed retries it stops using SSE and reads the cached job from IndexedDB every 5 seconds. It never polls the server for job status.

## HTTP middleware

The middleware chain runs in this order:

1. Request ID
2. Real IP
3. Request logger
4. Panic recovery
5. CORS
6. Rate limiter

CORS allows any origin and the methods `GET`, `POST`, `DELETE`, and `OPTIONS`. The rate limiter trusts `X-Forwarded-For` as sent.

The logger copies the request body and buffers the response body. For a long-lived SSE connection, that buffer grows for the life of the stream. The buffered response body is not used for anything beyond status logging.

## BLAST parameter whitelist

At startup the server runs `blastn -help`, `blastp -help`, `blastx -help`, `tblastn -help`, and `tblastx -help`, then parses the flag names from the output. If no output can be parsed, the server exits with a fatal error. It does not start with an empty whitelist.

The whitelist excludes these server-controlled flags:

- `query`
- `db`
- `outfmt`
- `num_threads`
- `out`

BLAST+ applies the last value when a flag appears twice. A user-supplied value for one of these flags could override the server's query file, database, output format, thread count, or output path.

Every other flag that the installed BLAST+ reports is allowed. User parameters are checked at submission and again in the worker before execution. Unknown keys return `400` and never enter the queue.

The server pins the query file, database, output format, and thread count. The tunable defaults are `-max_target_seqs 5000` and `-evalue 10`. The parser keeps at most 20 hits per database; the pool merges and trims to 200 hits overall.

## Rate limiting

The server uses a token bucket per IP address:

- Rate: 100 requests per second.
- Burst: 120 requests.
- Rejected request: `429 Too Many Requests` with `Retry-After: 5`.
- Cleanup: a sweep every 5 minutes removes entries idle for more than 10 minutes.

The limiter applies to every route, including static assets and SSE.

## Shutdown

On `SIGINT` or `SIGTERM`:

1. The HTTP server stops accepting connections and drains in-flight requests for up to 5 seconds.
2. `pool.Stop` cancels running and queued jobs, closes the queue, and waits up to 30 seconds for workers.
3. The process exits with `os.Exit(0)`.

The exit skips deferred cleanup, including the database watcher's `Stop`. That is harmless at process exit.

## Frontend

### Routes and URL state

`AppLayout` renders the header, the degraded banner, and the active page.

| Path | Page | URL parameters |
|---|---|---|
| `/` | Redirect to `/blast` | none |
| `/blast` | BLAST search, job list, results, alignment, spatial panel | `?job=<id>` selects a cached job |
| `/transcript` | Transcript lookup and region view | `?db=<name>&id=<id>` |
| `/settings` | Language, appearance, notifications, cache | none |
| any other path | Redirect to `/blast` | none |

Cross-page state lives in URL search parameters, not in a shared store.

- A BLAST hit links to `/transcript?db=<hit database>&id=<feature ID>`.
- Going back restores the BLAST page with `?job=<id>` from the history entry and loads the job from IndexedDB.
- Transcript URLs are bookmarkable and survive a refresh.
- Gene-family links and spatial gene links rewrite the `id` parameter, so browser history walks the lookup chain.
- Alignment-level transcript links are hidden for chromosome databases; the spatial panel is the intended path there.
- Changing the database on the transcript page drops the `id` parameter.

### Serving and SPA fallback

API routes are registered before the static catch-all, so they never reach it.

| Request | Behavior |
|---|---|
| Path maps to an embedded file | Served as is, with its content type |
| Extensionless path with no embedded file | Rewritten to `/` and served `index.html`; the client router takes over |
| Path with a file extension and no embedded file | `404` |
| Path whose extension is `.go` | `404` |

The `.go` guard exists because `//go:embed *` also matches this package's source files. The catch-all has no other exclusion list: new API routes inherit precedence automatically, and a new client route needs no server change. The corollary is that client routes must not contain a file extension.

### Tech stack

| Component | Library | Role |
|---|---|---|
| UI | React 18.3 | Component model |
| Build | Vite 6 | Development server, bundling, tree-shaking |
| Components | Ant Design 6.5 | Forms, tables, tags, cards, layout |
| Server data | TanStack Query 5 | Health, database list, request cache |
| Job updates | `EventSource` (SSE) | Server push for job progress |
| Client storage | IndexedDB | Job history and settings |

### Design system

Visual decisions live in `web/src/theme.ts` and `web/src/themeMode.tsx` as Ant Design v6 tokens. Theme tokens are the rule; a few caption colors in components are still literal values.

- Brand seed: teal `#0e7490` in light mode and `#2dd4bf` in dark mode. It drives `colorPrimary`, `colorInfo`, and `colorLink`.
- Status seeds: green, amber, and red, with brighter dark-mode variants.
- Neutrals: derived by Ant Design's `defaultAlgorithm` and `darkAlgorithm`, then read through `theme.useToken()`. The dark theme sets `colorBgBase` to `#0b1220` and `colorTextBase` to `#e5e7eb`.
- Shape: 8 px base radius, 12 px cards. The UI font stack is Google Sans Flex with Noto Sans SC fallback. Code and sequence surfaces use Fira Code.
- Nucleotide colors: IGV-style A green, C blue, G amber, T red, U pink, and a slate gap color. Each has a dark variant.
- `SequenceText` colors a string only when it contains nucleotide, IUPAC ambiguity, or gap characters and is at most 5,000 bases. Ambiguity codes use the muted ambiguous tone. Protein, other characters, and longer strings render as plain text. Consecutive same-color runs become one `<span>`.
- Icons: the brand mark is an inline SVG double helix. The favicon is a data-URI SVG. The interface uses no emoji as icons.

### Dark mode

The preference is `system`, `light`, or `dark`, and defaults to `system`. `ThemeModeProvider` stores it in `localStorage` under `helixblast_theme`.

An inline script in `index.html` resolves the preference before React mounts and sets `data-theme`, `color-scheme`, and the root background. That avoids a flash of the wrong theme. When the preference is `system`, a `matchMedia` listener updates the resolved mode when the operating system theme changes.

`getThemeConfig(resolved)` swaps `defaultAlgorithm` for `darkAlgorithm`. Components read neutrals through `theme.useToken()`, so they do not branch on the mode. The brand and nucleotide palettes have explicit light and dark values.

The header has a sun or moon toggle. The Settings page has a System, Light, Dark control under Appearance.

### IndexedDB-first data flow

IndexedDB is the source of truth for job state in the browser.

```text
Submit POST /api/v1/jobs
  -> response {job_id, status, queue_pos}
  -> saveJobMeta(idb)
  -> reloadLocalJobs(idb)
  -> set ?job=<id>
  -> loadCachedJob(idb)
  -> if terminal with result, skip SSE
  -> otherwise open EventSource

SSE stream
  -> queued, running
  -> terminal snapshot with result
  -> saveJobFull(idb)
  -> reloadLocalJobs(idb)
  -> server clears its result

Page reload
  -> loadJobs(idb); expired entries are deleted
  -> render the list from IndexedDB
  -> select a job; loadCachedJob(idb)
  -> if cached result exists, skip SSE
```

There is no server-side job list and no polling of the server. A job submitted in one browser is invisible in another. Non-job settings live in the same object store with type `setting` and are exempt from the 24-hour TTL.

### SSE resume and fallback

```text
EventSource onerror
  -> if the stream is not CLOSED, let the browser reconnect
  -> if CLOSED and the cached job is terminal, stop
  -> retry at most 10 times: 2s, 4s, 8s, ... capped at 30s
  -> after 10 failed retries, read IndexedDB every 5s
  -> never request job status from the server
```

### Server-side result lifecycle

```text
worker: SetResult -> SetStatus(success) -> notify
SSE:    send snapshot -> flush -> terminal? -> ClearResult
GET:    write snapshot -> terminal? -> ClearResult
```

`SetResult` runs before `SetStatus` so a snapshot cannot report success with a missing result. `ClearResult` sets the job's result to nil. The job record, status, and timestamps remain until the TTL pruner removes them.

### Spatial search

The spatial index stores gene, mRNA, CDS, and exon records sorted by start. The API returns only genes.

When a BLAST hit belongs to a database with `is_chromosome_db: true`, the frontend builds a query range from the hit. It starts with the first HSP returned by BLAST and adds later HSPs that fall within 20 kb of the current span. Each added HSP can extend the span, so a chain of HSPs can grow the window; the API's 1 Mb cap is the hard bound.

`SpatialSearch` binary-searches the start-sorted array, then scans backward within an 8 Mb window for overlapping genes and the nearest upstream gene. It reads forward for the first downstream gene. The backward window assumes no gene is longer than 8 Mb. Only returned features keep their decoded ID and type strings; the reader does decode the type of each candidate record while it filters for genes.

The response carries full indexed coordinates, never a clipped span. A gene that overlaps the query at one base is returned with its complete start and end.

### State management

The app uses local React state and the TanStack Query cache for server data. It does not use Redux or Zustand. The job list is `useState` backed by IndexedDB, not a query cache.
