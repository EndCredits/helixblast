# API Reference

All endpoints return JSON. All paths are under the server root.

## Health

```
GET /health
→ {
    "status": "healthy|degraded",
    "version": "0.1.0",
    "concurrent_capacity": 10,
    "storage_backend": "local"
  }
```

`status` is `degraded` when actual concurrent capacity < 5 (low CPU or memory). The frontend displays a warning banner in this state.

## Databases

```
GET /api/v1/databases
→ [
    {
      "name": "nt",
      "type": "nucleotide",
      "description": "NCBI Nucleotide",
      "last_updated": "2026-05-10"
    }
  ]
```

Returns all entries from `databases.yaml`. Paths and credentials are never exposed.

## Jobs

### Submit

```
POST /api/v1/jobs
Content-Type: application/json

{
  "fasta": ">seq1\nATGCGTACGTA",
  "program": "blastn",
  "db": "nt",
  "advanced_params": { "task": "megablast", "evalue": "1e-10" }
}

→ 201
{
  "job_id": "hxb-8f3a9c1d",
  "status": "queued",
  "queue_pos": 3
}
```

| Field | Required | Notes |
|-------|----------|-------|
| `fasta` | Yes | Valid FASTA format. Shell metacharacters rejected |
| `program` | Yes | `blastn`, `blastp`, `blastx`, `tblastn` |
| `db` | Yes | Matches `name` field in `databases.yaml` |
| `advanced_params` | No | Key-value map passed as `-key value` to BLAST+. Validated against whitelist |

Returns `429 Too Many Requests` when the job queue is full.

### List

```
GET /api/v1/jobs
→ [
    {
      "job_id": "hxb-8f3a9c1d",
      "status": "running",
      "queue_pos": 0,
      "program": "blastn",
      "database": "nt",
      "created_at": "2026-05-16T12:00:00Z"
    }
  ]
```

Lists all jobs. `queue_pos` is 0 for running/completed jobs, ≥1 for queued jobs.

### Detail

```
GET /api/v1/jobs/{id}
→ {
    "job_id": "hxb-8f3a9c1d",
    "status": "success",
    "program": "blastn",
    "database": "nt",
    "created_at": "...",
    "updated_at": "...",
    "result": {
      "job_id": "hxb-8f3a9c1d",
      "status": "success",
      "database": "nt",
      "program": "blastn",
      "results": [
        {
          "subject_id": "NR_076022.1",
          "identity": 98.5,
          "coverage": 100.0,
          "e_value": "1.2e-50",
          "total_score": 1245.2,
          "alignments": [
            {
              "query_start": 1, "query_end": 320,
              "subject_start": 45, "subject_end": 364,
              "query_seq": "ATGC...TCGA",
              "subject_seq": "ATG-...TCGA"
            }
          ]
        }
      ]
    }
  }
```

Results contain the top 20 hits sorted by bitscore. Each hit contains HSP alignments with the query and subject sequences. Coverage is derived from the `qcovs` BLAST output column. Identity, e-value, and bitscore reflect the best HSP per hit.

### Cancel

```
DELETE /api/v1/jobs/{id}
→ 202 { "status": "cancelling" }
```

Soft cancel: sets a cancellation flag. BLAST runs to completion but the result is discarded. Already-terminal jobs return `400`.

### Events (SSE)

```
GET /api/v1/jobs/{id}/events
Content-Type: text/event-stream

data: {"job_id":"...","status":"running","progress":"BLAST search in progress"}

data: {"job_id":"...","status":"success","result":{...}}
```

Server-Sent Events stream. Pushes on every status change (not on a timer). Terminates when the job reaches a terminal state (`success`, `failed`, `cancelled`). The frontend uses `EventSource` with exponential backoff reconnection and 5s HTTP polling fallback after 10 failures.

## Transcript Lookup

```
GET /api/v1/transcripts?db=<name>&transcript=<id>
→ {
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
      "exons": [{"start": 11401, "end": 11811}, ...],
      "cdss": [{"start": 11401, "end": 11811}, ...]
    },
    "related": {
      "transcripts": ["g1.t1"],
      "cdss": ["g1.t1.CDS1"],
      "exons": ["g1.t1.exon1"]
    }
  }
```

Resolution order:
1. Local `transcript.index_path` in `databases.yaml` → direct disk read with `f.Seek()` for O(1) chromosome positioning
2. `database.worker_url` in `config.yaml` → proxy to Cloudflare Worker
3. Neither → `503 Service Unavailable`

Any GFF3 ID works — gene, mRNA, CDS, or exon. The response includes the full gene family for inter-query navigation.

The `sequence` field contains the genome scan from `scan_start` to `scan_end` (5kb upstream + gene body). Five regions are sliced client-side:
- **5 kb Upstream**: `seq[0 : start - scan_start]`
- **2 kb Upstream**: last 2kb of the upstream region
- **Gene Body**: `seq[(start - scan_start) : ]`
- **mRNA**: exons spliced from gene body
- **CDS**: coding regions spliced from gene body
