# Configuration

HelixBLAST reads two YAML files at startup. `config.yaml` controls the server and job pool. `databases.yaml` lists the BLAST databases and their transcript sources. HelixBLAST does not read configuration from environment variables. `blast.path` can be empty to use `PATH`.

## config.yaml

### server

```yaml
server:
  port: 8080
```

`port` defaults to `8080`. A value of 0 also selects the default; other values must be between 1 and 65535. The same port serves the API and the embedded frontend.

### jobs

```yaml
jobs:
  result_ttl_hours: 24
```

| Field | Default | Description |
|---|---|---|
| `result_ttl_hours` | `24` | How long a terminal job stays in the in-memory registry. The pruner runs every 10 minutes, so a job can remain for up to 10 minutes past its TTL. After removal, the ID returns `404`. A value of 0 uses the default; negative values are rejected. |

The server never writes job results to disk. The retention window only bounds memory use. Browsers keep their own copy in IndexedDB.

### blast

```yaml
blast:
  path: ""              # BLAST+ directory; empty means use PATH
  max_jobs: 20          # concurrency cap and queue capacity
  cpu_per_job: 2        # threads per BLAST process
```

| Field | Default | Description |
|---|---|---|
| `path` | `""` | Directory that contains `blastn`, `blastp`, and the other BLAST+ executables. Empty means search `PATH`. The server exits at startup if it cannot find BLAST+ or build the parameter whitelist. |
| `max_jobs` | `20` | Upper bound on concurrent BLAST processes. The job queue also holds up to this many waiting jobs. The sample configurations set `5`. |
| `cpu_per_job` | `2` | Value passed to BLAST+ as `-num_threads`. More threads per job can reduce the number of concurrent jobs. |

### Resource detection

At startup, the server computes the actual concurrency:

```text
cpu_limit    = max(1, NumCPU / cpu_per_job)
memory_limit = available RAM < 2 GB  -> 2
               available RAM < 4 GB  -> 5
               otherwise             -> 20
actual       = min(max_jobs, cpu_limit, memory_limit)
```

On Linux, `memory_limit` uses `MemAvailable` from `/proc/meminfo`. On other operating systems, it is fixed at 4. Because the actual concurrency is capped by that value, a non-Linux host always reports `degraded`, even when it has many cores and enough RAM.

If `actual` is less than 5, `/health` reports `status: "degraded"` and the frontend shows a dismissible warning. Jobs still run; only throughput is reduced. A `max_jobs` value below 5 also marks the server degraded.

### database

```yaml
database:
  config_path: ./databases.yaml
  worker_url: ""        # Cloudflare Worker base URL
```

| Field | Default | Description |
|---|---|---|
| `config_path` | `./databases.yaml` | Path to the database manifest. Relative paths resolve from the process working directory, not from the location of `config.yaml`. |
| `worker_url` | `""` | Base URL of the Cloudflare Worker used for transcript sequence extraction. The server appends `/sequence`. |

Local FASTA sources take priority over the Worker. The Worker is used only when a database configures neither `fasta_dir` nor `fasta_file` and `worker_url` is set. This is a deployment-mode fallback, not an error fallback: a missing chromosome file in a configured `fasta_dir` does not spill over to the Worker.

The Docker image has no `WORKDIR` and runs from `/`. If you use the image's copied manifest, set `database.config_path` to `/etc/helixblast/databases.yaml`; the sample value `./databases.yaml` resolves to `/databases.yaml`. The image also expects `config.yaml` and `databases.yaml` in the build context. Both files are gitignored, so create them from the examples before `docker build`.

### Validation and reload

The server applies defaults before validation. A zero value therefore means "use the default", not "disable": `port`, `result_ttl_hours`, `max_jobs`, and `cpu_per_job` must be positive after defaults are applied. Ports outside 1 to 65535 are rejected.

Both YAML files are decoded with `yaml.Unmarshal`, so unknown keys are ignored. A typo in a field name falls back to the default or zero value instead of producing an error.

`config.yaml` is read once. `databases.yaml` is watched and can reload without a restart. A missing or unparseable `config.yaml`, or an invalid `databases.yaml` at startup, aborts the process.

The server also requires BLAST+. A non-empty `blast.path` must exist, or `PATH` must contain `blastn`. The server runs `-help` for all five programs and exits if no flag list can be parsed.

### Why the server has no storage layer

Job state lives in memory. Earlier versions included local and S3/MinIO storage backends, presigned URLs, and a janitor for expired artifacts. The IndexedDB-first design made that subsystem unused, so it was removed. Genome FASTA is read from the local filesystem or fetched through the Cloudflare Worker and R2. GFF3 indexes are always read from the local filesystem; the Worker never serves an index. The only remaining retention setting is `jobs.result_ttl_hours`.

## databases.yaml

Each entry declares one BLAST database and, optionally, a transcript lookup source.

```yaml
databases:
  - name: "arachis-9102"
    type: "protein"
    path: "/path/to/blastdb/YZ9102-prot"
    description: "Peanut YZ9102 genome annotation"
    last_updated: "2026-05-16"
    is_chromosome_db: false
    transcript:
      index_path: "/data/gff3/arachis-9102.index.bin"
      fasta_dir: "/data/genome/arachis-9102/"
      # fasta_file: "/data/genome/arachis-9102.fa"
```

| Field | Required | Description |
|---|---|---|
| `name` | Yes | Name used in API requests and shown in the UI. In Worker mode, it also names the R2 directory unless `transcript.source` overrides it. Duplicate names are not rejected; `Lookup` returns the first match. |
| `type` | Yes | `nucleotide` or `protein`. Other values are rejected when the file loads. |
| `path` | Yes | Path prefix of the BLAST database, before `.phr`, `.pin`, `.psq` for protein databases or `.nhr`, `.nin`, `.nsq` for nucleotide databases. The path is passed to BLAST+ as given; it is not resolved or checked for existence. |
| `description` | No | Returned by `/api/v1/databases`. The bundled database selector does not display it. |
| `last_updated` | No | Returned by `/api/v1/databases`. The bundled database selector does not display it. |
| `is_chromosome_db` | No | When `true`, the frontend shows the spatial panel for hits from this database. The server does not enforce this flag. Spatial lookup still requires `transcript.index_path`; without one the endpoint returns `503`. |
| `transcript` | No | Enables transcript lookup and spatial search for this database. |

The loader checks that `name`, `type`, and `path` are present. It does not validate the transcript section, file existence, or duplicate names. A missing or misspelled top-level `databases:` key decodes to an empty list and passes validation, so the server runs with zero databases. On reload, that empty list replaces the current one. An invalid file at initial startup aborts the process; only a reload keeps the old list.

YAML keys that are not recognized are ignored. A typo in a field name therefore falls back to the zero value or the default instead of producing an error.

### transcript section

| Field | Required | Description |
|---|---|---|
| `index_path` | Yes | Path to the local GFF3 index. Transcript and spatial lookup return `503` when it is empty, even if `worker_url` is set. Use a `.bin` file for the runtime path and build it with `helixblast-index`. `.json` and `.json.gz` files are also supported and are decoded once, then cached by path. If the configured path is JSON, the server prefers a sibling `.bin` file whenever one exists; it does not compare timestamps, so a stale or corrupt `.bin` fails instead of falling back to JSON. |
| `fasta_dir` | No | Directory of per-chromosome FASTA files named `<chromosome>.fa`. The server scans the file from the beginning to the requested range; it does not use `fasta_index` for this mode. It does not read `.fa.gz` files here. |
| `fasta_file` | No | One multi-FASTA file that contains every chromosome. Used when the chromosome file is not present in `fasta_dir`, or as the only local source when `fasta_dir` is empty. This path uses `fasta_index` byte offsets when the index provides them. |
| `source` | No | Worker mode only. Database name used for the R2 path instead of `name`. Set it when a protein and a nucleotide BLAST database share one genome assembly. |

If `fasta_dir` and `fasta_file` are both empty and `worker_url` is not set, transcript lookup still returns coordinates and metadata, but `sequence` is an empty string.

### Sharing one genome between databases

```yaml
databases:
  - name: "arachis-9102-prot"
    type: "protein"
    path: "/path/to/blastdb/prot"
    transcript:
      index_path: "/data/gff3/arachis-9102.index.bin"
      source: "arachis-9102"
  - name: "arachis-9102-nuc"
    type: "nucleotide"
    path: "/path/to/blastdb/nuc"
    transcript:
      index_path: "/data/gff3/arachis-9102.index.bin"
      source: "arachis-9102"
```

With `source`, both entries read from `fasta/arachis-9102/` in R2. Without it, the Worker looks under each database's own name.

### Hot reload

The server watches the directory that contains `databases.yaml` with fsnotify. It reloads only when that exact path receives a write or create event; remove and rename events are ignored. `config.yaml` is read once at startup and is not watched.

A reload validates only `name`, `type`, and `path`. If the file fails to parse or fails that validation, the old database list stays active and the error is logged.

## Build a transcript index

Build the runtime index directly from GFF3 and genome FASTA:

```bash
make build-index
./helixblast-index --gff3 input.gff3 --fasta genome.fa --out db-name.index.bin
```

Add `--json` to keep an intermediate JSON index for inspection or for `verify`:

```bash
./helixblast-index --gff3 input.gff3 --fasta genome.fa \
  --out db-name.index.bin --json db-name.index.json.gz
```

The binary index contains:

| Section | Content |
|---|---|
| `entries` | Every GFF3 ID whose Parent chain reaches a gene, mRNA, or transcript, plus every `Name` alias. Gene, mRNA, and transcript rows use their own coordinates; child features use the nearest annotated ancestor. Rows without an ID, or with a Parent chain that does not resolve, are omitted. Multi-parent values such as `Parent=t1,t2` are not split and do not resolve. |
| `families` | Gene to transcript, CDS, and exon ID lists. |
| `coords` | Exon and CDS coordinate arrays per transcript. These are keyed by mRNA ID; a gene, CDS, or exon query falls back to the first transcript in the family that has coordinates. |
| `fasta_index` | Byte offset of each chromosome header in the genome FASTA. |
| `spatial` | Per-chromosome features sorted by start position. The index stores genes, mRNA, CDS, and exon records; the spatial API returns genes only. |

The Cloudflare Worker does not read this index. It reads FASTA from R2 and does no GFF3 parsing.

## Split FASTA for the Worker

The Worker expects one gzip file per chromosome:

```bash
./worker/scripts/split_fasta.sh /path/to/genome.fa output_dir/
# output_dir/Chr01.fa.gz, output_dir/Chr02.fa.gz, ...
```

The script streams each record through `gzip`, then checks every output with `gzip -t`. For local `fasta_dir` mode, use uncompressed `<chromosome>.fa` files instead; the server does not read `.fa.gz` from `fasta_dir`.

## Convert an existing JSON index

`helixblast-prepare` converts a JSON index when the GFF3 source is no longer available:

```bash
make build-prepare
./helixblast-prepare --json db-name.index.json.gz --out db-name.index.bin
```

Place the `.bin` next to the JSON file. The server detects the sibling file through `LoadIndexAuto`, so no configuration change is needed. The binary index has no decoded Go heap cost: the operating system pages in the file on demand and releases the pages when the reader closes. The JSON path keeps one decoded copy per index after the first load.
