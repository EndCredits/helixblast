# Changelog

Notable changes to HelixBLAST. The project does not publish versioned releases yet, so entries are grouped by date in `YYYY-MM-DD` form. The most recent changes appear first.

## 2026-10-18

### Fixed

- Minus-strand transcript lookups now extract the 5 kb upstream flank from the high-coordinate side (`[start, end + 5000]`) instead of the low-coordinate side. `scan_end` reports the last base actually extracted, including at chromosome boundaries.
- The transcript page slices upstream, gene body, mRNA, and CDS from the returned sequence using the strand. Exons and CDS are spliced in transcript order, descending by genomic start on the minus strand.
- Alignment position labels step down for minus-strand query and subject HSPs instead of counting upward.
- Added Go coverage for local and Worker minus-strand windows and Node tests for transcript region slicing and alignment position stepping.

## 2026-09-11

### Changed

- `/api/v1/spatial` returns genes only. The `overlapping` array lists genes whose span intersects the query range, and `upstream` and `downstream` are the nearest genes on each side. mRNA, CDS, and exon records stay in the index but are no longer returned; open Transcript Lookup to inspect them.
- Automatic spatial search uses a smaller window. The frontend starts with the first HSP in the BLAST output and adds later HSPs that fall within 20 kb of the current span. A low-complexity query can produce HSPs megabases apart; the old union of every HSP produced an 87 Mb window and 24,466 features in one observed case. The API also rejects windows wider than 1 Mb with `400`.
- `POST /api/v1/jobs` deduplicates the `dbs` array while preserving order. Sending `["nt","nr","nt"]` runs two searches, not three.
- The database selector no longer renders a duplicate row of removable tag chips above the multi-select. Selected databases appear only in the select control.
- The worker pool refuses submissions during shutdown with `503 Service Unavailable` instead of panicking on a closed queue channel.
- i18next plural keys use the v4 `_one` and `_other` suffixes. The old `_plural` keys never resolved for counts greater than one.
- The build context now uses `.dockerignore` to keep `.git`, `node_modules`, and build artifacts out of image builds.

### Fixed

- `index.Open` validates every header-derived section range against the file size before any lookup. Truncated or corrupted `.bin` files now fail at open. The header is copied to the heap before any error path can unmap it, which removes a use-after-unmap in the bad-magic and unsupported-version branches.
- The API no longer races with the pool during shutdown.
- The spatial and transcript readers agree on gene-only filtering and nearest-gene selection. Both are cross-checked against a naive reference reader.

### Security

- Job IDs are 128-bit values: `hxb-` followed by 32 hexadecimal characters from 16 random bytes. The ID is the only credential on unauthenticated job endpoints, so 32-bit IDs were too easy to guess and could collide in the registry.
- The server refuses to start when it cannot build the BLAST+ parameter whitelist. A missing whitelist used to allow every parameter.
- `megablast` is no longer in the valid program list. Modern BLAST+ has no `megablast` executable; it is a `blastn` task.

### Removed

- The `internal/storage` and `internal/janitor` packages, including the local and S3/MinIO backends, presigned URLs, and the TTL janitor. Results are not persisted on the server.
- The `storage` and `s3` sections of `config.yaml`. The registry TTL now lives in `jobs.result_ttl_hours`.
- Dead code left over from the removed job-list endpoint: `Pool.List`, `queuePosUnsafe`, `doneCh`, `ExecConfig.Timeout`, and unused theme and i18n exports.

## 2026-08-31

### Added

- Routable `/blast`, `/transcript`, and `/settings` pages under a shared layout.
- Dark mode with `system`, `light`, and `dark` preferences. The preference is stored in `localStorage` and applied before first paint.
- A JSON index cache. Decoded JSON readers are cached by path and invalidated when the file's modification time or size changes.
- Range queries for `/api/v1/spatial`, with `start` and `end` in addition to the legacy `pos` point query.

### Changed

- The frontend uses an Ant Design v6 token theme: one teal brand seed, slate neutrals, and 8/12 px radii. The IGV-style nucleotide palette applies to sequence data only.
- Transcript lookup opens as a separate page. BLAST hits and spatial gene IDs navigate to `/transcript?db=<name>&id=<id>`.
- Spatial search is again an auxiliary panel on the BLAST page. It appears below the alignment when the selected hit belongs to a database with `is_chromosome_db: true`.
- The transcript lookup button on alignment hits is hidden for chromosome databases, where spatial search is the intended path.
- The binary index reader exposes `SpatialSearch(chr, start, end)` instead of copying a whole chromosome.

### Fixed

- Base coloring in alignments.
- Chromosome range extraction for the test fixtures.
- The JSON index path builds a per-request entry cache instead of decoding the whole index for every lookup.

### Removed

- The old whole-chromosome spatial copy and the dead v1 `SpatialLookup` path.

## 2026-08-21

### Added

- SPA deep-link fallback. Extensionless paths without an embedded file serve `index.html`; paths with a file extension keep a real `404`.
- Registry TTL pruning for terminal jobs. The pruner runs every 10 minutes and removes jobs older than `jobs.result_ttl_hours`.
- `404 Not Found` for job IDs that were pruned or never existed.

### Changed

- Job snapshots are plain `JobSnapshot` values. Responses no longer copy a `Job` that embeds a mutex.
- Queue positions are written through the locked `SetQueuePos` method.

### Fixed

- A race when two callers updated the queue position.
- A memory leak in job lifetime handling.
- Data races found by concurrent-access tests.

## 2026-08-07

### Added

- `helixblast-index`, a Go tool that builds the binary index directly from GFF3 and genome FASTA. It replaces the Node.js `prepare.js` workflow.
- English and Chinese UI strings with i18next.

### Changed

- The frontend uses Ant Design v6.
- Invalid request parameters return `400` instead of being passed to BLAST+.

### Fixed

- npm audit findings and Ant Design lint issues.

## 2026-08-06

### Changed

- The binary index uses xxh3 instead of FNV-1a. The format version is now 2, so version 1 indexes must be rebuilt.

## 2026-06-26

### Changed

- The browser's IndexedDB is the source of truth for job state. The server clears a result after SSE delivery or after the first completed `GET /api/v1/jobs/{id}`.
- `GET /api/v1/jobs` is removed. It exposed every running job to any caller. `GET /api/v1/jobs/{id}` remains for clients that know the ID.
- The API converts `template` to `advanced_params.task` when `task` is not already set.
- The worker sets the result before it sets the terminal status, so an SSE snapshot cannot contain `status: "success"` with a null result.
- Cancel re-reads the job status before it changes it, and the worker checks the cancellation flag after it installs the cancel function.

## 2026-05-20

### Added

- Memory-mapped binary GFF3 index, built by `helixblast-prepare` from an existing JSON index.
- `verify`, which builds a temporary binary index and compares entries, families, coords, and FASTA offsets against the JSON source.
- `make build-verify`.

### Changed

- The project is licensed under MIT, with documentation under CC BY-SA 4.0.

### Fixed

- Spatial lookup returns an empty list instead of `null` when no features match.

## 2026-05-17

### Added

- Multi-database BLAST. `POST /api/v1/jobs` accepts `dbs`, runs each database sequentially, and tags every hit with its source database.
- Spatial search for chromosome databases, including automatic lookup from a BLAST hit.
- Browser-local result cache in IndexedDB with a 24-hour TTL.
- Streaming FASTA split script that writes one gzip file per chromosome and verifies each archive with `gzip -t`.
- Docker image based on Debian with NCBI BLAST+.

### Changed

- The job list sorts by creation time, newest first, and supports collapsing and deleting old jobs.

## 2026-05-16

### Added

- Initial HelixBLAST server: BLAST job submission, job status, SSE updates, and a web UI.
- Transcript lookup backed by a GFF3 index and local or remote genome FASTA.
- Token-bucket rate limiting: 100 requests per second per IP with a burst of 120.
- Graceful shutdown on `SIGINT` and `SIGTERM`.
- Docker packaging.
- NCBI-style parameter presets and automatic scrolling to results.
- SSE reconnect with backoff instead of polling immediately after page load.
