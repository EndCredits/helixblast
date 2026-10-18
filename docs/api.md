# API reference

API endpoints return JSON unless noted. All paths are relative to the server root. The server applies a token-bucket limit of 100 requests per second per IP with a burst of 120; a limited request returns `429` with `Retry-After: 5`.

## Health

```http
GET /health
```

```json
{
  "status": "healthy",
  "version": "0.1.0",
  "concurrent_capacity": 10
}
```

`status` is `healthy` or `degraded`. The server reports `degraded` when the effective job concurrency is less than 5, including when `blast.max_jobs` itself is below 5. `concurrent_capacity` is the number of BLAST jobs that can run at once. The frontend shows a dismissible warning when the status is `degraded`.

## Databases

```http
GET /api/v1/databases
```

```json
[
  {
    "name": "nt",
    "type": "nucleotide",
    "description": "NCBI Nucleotide",
    "last_updated": "2026-05-10",
    "is_chromosome_db": false
  }
]
```

The response contains every entry from `databases.yaml`, without paths or credentials. `is_chromosome_db` tells the frontend whether to run automatic spatial lookup for hits from that database. The server does not enforce the flag on the spatial endpoint.

## Jobs

Job records live in server memory only. The server does not write results to disk.

There is no endpoint that lists all jobs. Clients submit a job, keep its ID, and either follow the SSE stream or request the job by ID. Job IDs are 128-bit random values with the `hxb-` prefix. The server has no authentication, so the ID is the only credential for its job; treat it as a secret.

Errors use a JSON body with one `error` field.

### Lifecycle and retention

A job passes through these states:

```text
pending -> queued -> running -> success
                              -> failed
         -> cancelled
```

A terminal job (`success`, `failed`, or `cancelled`) stays queryable for `jobs.result_ttl_hours` after its last state change. The registry pruner runs every 10 minutes and removes older terminal jobs. After that, the ID returns `404 Not Found`. A server restart clears the registry immediately.

Result delivery is at most once. The terminal SSE event carries the result when SSE is the delivery path; otherwise, the first `GET /api/v1/jobs/{id}` after completion returns it. The server drops its copy after either path. Later reads within the retention window return status and metadata without `result`. Clients that need the result must store it when it arrives; the bundled frontend uses IndexedDB.

### Submit

```http
POST /api/v1/jobs
Content-Type: application/json
```

```json
{
  "fasta": ">seq1\nATGCGTACGTA",
  "program": "blastn",
  "dbs": ["nr", "nt"],
  "db": "nt",
  "template": "megablast",
  "advanced_params": {
    "task": "megablast",
    "evalue": "1e-10"
  }
}
```

```json
{
  "job_id": "hxb-8f3a9c1d5e7b2046af01be39c72d5408",
  "status": "queued",
  "queue_pos": 3
}
```

| Field | Required | Description |
|---|---|---|
| `fasta` | Yes | Query in FASTA format. For a valid program, the server checks the header and the sequence alphabet. It does not pass the query through a shell. |
| `program` | Yes | `blastn`, `blastp`, `blastx`, `tblastn`, or `tblastx`. `megablast` is a `blastn` task, not a program. |
| `dbs` | Yes* | Array of database names. Takes precedence over `db`. Duplicate names are removed while first-seen order is preserved. |
| `db` | Yes* | Single database name. Used when `dbs` is absent or empty. |
| `template` | No | Shorthand for `advanced_params.task`. The server sets `task` only if the map does not already contain it. |
| `advanced_params` | No | Map of BLAST+ parameters. Keys and values must be strings. The server validates every key against the whitelist built from `-help` output. Unknown keys return `400`. |

*Provide `dbs` or `db`.

The server checks that every database name exists in `databases.yaml`. It does not validate the `program` string against the list at submission time; an unsupported name is accepted and fails when the job tries to run BLAST+.

The server always adds `-outfmt 6 qseqid sseqid pident qcovs evalue bitscore qstart qend sstart send qseq sseq`, `-max_target_seqs 5000`, `-evalue 10`, and `-num_threads <blast.cpu_per_job>`. An advanced parameter can override `evalue` or `max_target_seqs` because those flags are in the whitelist; `query`, `db`, `outfmt`, `num_threads`, and `out` cannot be overridden.

The whitelist is the union of the flags reported by all five programs. A flag that is valid for a different program passes submission and fails when BLAST+ runs. `blastx` and `tblastx` are classified as protein programs for FASTA validation, so the nucleotide alphabet is not enforced for them. Characters outside the protein alphabet, such as `B`, are rejected.

When `dbs` contains several databases, one job runs BLAST against each database in sequence. Hits are merged, sorted by bitscore, and tagged with their source database. An error in one database is recorded and does not stop the remaining databases. One 2-hour timeout covers the whole job, not each database.

Responses:

- `201 Created` when the job enters the queue.
- `400 Bad Request` for malformed JSON, missing fields, invalid FASTA, an unknown parameter, or an unknown database.
- `429 Too Many Requests` when the queue is full.
- `503 Service Unavailable` when the server is shutting down.

### Detail

```http
GET /api/v1/jobs/{id}
```

```json
{
  "job_id": "hxb-8f3a9c1d5e7b2046af01be39c72d5408",
  "status": "success",
  "program": "blastn",
  "database": "nr,nt",
  "created_at": "2026-05-17T10:00:00Z",
  "updated_at": "2026-05-17T10:02:31Z",
  "result": {
    "job_id": "hxb-8f3a9c1d5e7b2046af01be39c72d5408",
    "status": "success",
    "database": "nr,nt",
    "databases": ["nr", "nt"],
    "program": "blastn",
    "errors": [
      {
        "database": "nt",
        "error": "blast execution failed: ..."
      }
    ],
    "results": [
      {
        "subject_id": "NR_076022.1",
        "database": "nr",
        "identity": 98.5,
        "coverage": 100.0,
        "e_value": "1.2e-50",
        "total_score": 1245.2,
        "alignments": [
          {
            "query_start": 1,
            "query_end": 320,
            "subject_start": 45,
            "subject_end": 364,
            "query_seq": "ATGC...TCGA",
            "subject_seq": "ATG-...TCGA"
          }
        ]
      }
    ]
  }
}
```

A job that is still running returns `queue_pos`, `updated_at`, and `progress` when a progress message is set. `queue_pos` is 0 once the job starts. A failed job returns `error` and no result. A cancelled job returns `status: "cancelled"`.

Each database contributes at most 20 hits. The merged list is sorted by `total_score` and trimmed to 200 hits. Each hit keeps every HSP that BLAST+ returned for that subject. `coverage` comes from the `qcovs` column. `identity`, `e_value`, and `total_score` come from the best HSP for that hit.

`errors` is present when some databases failed but at least one succeeded. If every database fails, the job state becomes `failed` and the top-level `error` field contains the combined message. No `result` object is produced in that case.

`404 Not Found` means the ID was never submitted, was pruned, or the server restarted.

The result is delivered at most once. The server clears its copy after the first terminal write attempt, even if the client disconnected, and it does not replay SSE events. If the terminal event is lost, a later `GET` returns the job metadata without `result`.

### Cancel

```http
DELETE /api/v1/jobs/{id}
```

```json
{
  "status": "cancelling"
}
```

Returns `202 Accepted`. The server sets a cancellation flag and cancels the job context.

- A queued or pending job moves to `cancelled` immediately.
- A running job's BLAST process is terminated. The worker marks the job `cancelled` after it observes the cancellation.
- A terminal job returns `400 Bad Request`.
- An unknown or pruned ID returns `404 Not Found`.

### Events (SSE)

```http
GET /api/v1/jobs/{id}/events
Content-Type: text/event-stream
```

```text
data: {"job_id":"...","status":"running","progress":"BLAST against nr ..."}

data: {"job_id":"...","status":"success","result":{...}}
```

The server pushes an event when a job's status, progress, or result changes. The stream ends when the job reaches `success`, `failed`, or `cancelled`. The terminal event carries the result when one exists. Delivery is at most once: the server clears its copy after the terminal send attempt, and there is no event ID or replay.

The frontend opens an `EventSource` when it selects a job. On a connection error, it retries with exponential backoff, starting at 2 seconds and capped at 30 seconds. After 10 failed retries, it stops using SSE and reads the cached job from IndexedDB every 5 seconds. It does not poll the server for job status.

## Transcript lookup

```http
GET /api/v1/transcripts?db=<name>&transcript=<id>
```

```json
{
  "transcript_id": "g1.t1.CDS1",
  "database": "arachis-9102",
  "chromosome": "Chr05",
  "start": 11401,
  "end": 27453,
  "strand": "+",
  "type": "CDS",
  "gene_id": "g1",
  "scan_start": 6401,
  "scan_end": 27453,
  "sequence": "ATGC...TAGC",
  "regions": {
    "exons": [
      {
        "start": 11401,
        "end": 11811
      }
    ],
    "cdss": [
      {
        "start": 11401,
        "end": 11811
      }
    ]
  },
  "related": {
    "transcripts": ["g1.t1"],
    "cdss": ["g1.t1.CDS1"],
    "exons": ["g1.t1.exon1"]
  }
}
```

`db` and `transcript` are required. A missing parameter returns `400`. The server resolves the ID through `transcript.index_path` for the requested database. The index is always local, even when a Cloudflare Worker supplies the sequence. The index accepts gene, mRNA, CDS, and exon IDs, plus `Name` aliases. IDs whose Parent chain does not reach a gene, mRNA, or transcript are not indexed and return `404`.

Sequence sources are tried in this order:

1. `transcript.fasta_dir` reads `<chromosome>.fa` from the directory.
2. `transcript.fasta_file` reads the chromosome from a multi-FASTA file.
3. `database.worker_url` fetches the sequence from the Cloudflare Worker, but only when neither local source is configured.

If `index_path` is empty, the endpoint returns `503 Service Unavailable` before it looks at any sequence source. A missing ID, an unreadable index, or a missing chromosome in `fasta_file` returns `404`. A Worker failure returns `502 Bad Gateway`.

If a local source is configured, the server does not fall back to the Worker. With only `fasta_dir`, a missing chromosome file produces a response with an empty `sequence`. With `fasta_file`, a missing chromosome returns `404`.

`sequence` is the forward genomic window from `scan_start` to `scan_end`, reverse-complemented when `strand` is `-`. `scan_start` is the feature start minus 5,000, clamped at 1; `scan_end` is the feature end. The numeric fields stay genomic coordinates and do not index into the string for minus-strand features.

For plus-strand features, the first `start - scan_start` bases are the 5 kb upstream region. For minus-strand features, the window covers the low-coordinate side of the gene, which is the 3' side; after the reverse complement, the gene body comes first and that flank is at the end of the string.

The bundled frontend slices five regions from the string:

- 5 kb upstream: `sequence[0 : start - scan_start]`
- 2 kb upstream: the last 2 kb of the 5 kb upstream region
- Gene body: `sequence[start - scan_start :]`
- mRNA: exons spliced from the gene body
- CDS: coding regions spliced from the gene body

That layout assumes the plus strand. For minus-strand features, the five labels do not match the biological direction.

`regions` comes from the queried transcript when the index has coordinates for it. For a gene, CDS, or exon query, the server falls back to the first transcript in the gene family that has coordinates, so the exon and CDS arrays can belong to another transcript. `regions` and `related` are omitted when the index has no matching data.

## Spatial lookup

```http
GET /api/v1/spatial?db=<name>&chr=<chr>&start=<n>&end=<n>
GET /api/v1/spatial?db=<name>&chr=<chr>&pos=<n>
```

`db` and `chr` are required. Provide either `start` and `end` or `pos`. `pos` is a point query with `start == end`. Reversed bounds are normalized. The response carries the normalized range. Coordinates are 1-based integers and must be positive; invalid values return `400`.

```json
{
  "chromosome": "Chr01",
  "start": 17146608,
  "end": 17148002,
  "overlapping": [
    {
      "start": 17146608,
      "end": 17148002,
      "id": "arahy.Tifrunner.gnm2.ann2.Ah01g000100",
      "type": "gene"
    }
  ],
  "upstream": {
    "start": 11401,
    "end": 11811,
    "id": "...Ah01g000000",
    "type": "gene"
  },
  "downstream": {
    "start": 22075,
    "end": 27453,
    "id": "...Ah01g000200",
    "type": "gene"
  }
}
```

The endpoint is gene-centric:

- `overlapping` lists genes whose span intersects the query range. A range can overlap several genes, for example when it crosses two loci.
- `upstream` is the nearest gene that ends before the range.
- `downstream` is the nearest gene that starts after the range.
- mRNA, CDS, and exon records are excluded. Use Transcript Lookup with a gene ID to inspect its family.

Every returned gene keeps its full indexed span. A hit that covers only the 3' end of a gene still reports the gene's complete coordinates. `upstream` and `downstream` are independent of `overlapping`: they describe the nearest gene outside the range and are present even when the range overlaps one or more genes. They are omitted when no gene exists on that side.

The endpoint requires `transcript.index_path` for the database and returns `503` when it is empty. It returns `404` for an unknown database or a chromosome that is not in the spatial index, and `500` when the index cannot be opened. Windows wider than 1 Mb return `400`. The upstream search is bounded to 8 Mb; a gene longer than that could be missed.

The frontend enables automatic lookup only when the selected hit's database has `is_chromosome_db: true`. It starts with the first HSP in the BLAST output and adds later HSPs that fall within 20 kb of the current span. Each added HSP can extend the span, so a chain of HSPs can grow the window; the 1 Mb cap is the hard bound. This keeps low-complexity queries from producing chromosome-scale windows. The server does not require the flag; it only checks that the index has a spatial section.

## Offline cache

The bundled frontend stores jobs in IndexedDB under database `helixblast` and object store `cache`. Each job entry expires after 24 hours; `loadJobs` deletes expired entries on page load. Terminal entries appear with a `local` tag in the job list.

The job list and results come from IndexedDB. The server keeps no results after SSE delivery or the first completed `GET /api/v1/jobs/{id}`. Each browser has its own workspace; jobs do not sync across devices.
