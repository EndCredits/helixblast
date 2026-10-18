# HelixBLAST

HelixBLAST is a web service for NCBI BLAST+ searches. It queues BLAST jobs, streams status and results to the browser over Server-Sent Events (SSE), and resolves hits against GFF3 annotations for transcript and spatial lookups.

The server is a single Go binary with an embedded React frontend. NCBI BLAST+ must be installed on the host, either in `PATH` or in the directory set by `blast.path`.

## Features

- BLAST search. Submit nucleotide or protein queries, follow queued and running jobs, and inspect alignments in the browser.
- Multiple databases. Search several BLAST databases in one submission. The job worker runs each database in turn, merges the hits, sorts them by bitscore, and tags each hit with its source database. Errors from one database do not stop the others.
- Transcript lookup. Resolve a GFF3 gene, mRNA, CDS, or exon ID to genomic coordinates and extract the sequence from a local FASTA file or a Cloudflare Worker backed by R2. The lookup runs on its own `/transcript` page.
- Spatial search. For chromosome databases, resolve a BLAST hit to the genes that overlap its range and the nearest genes on each side.
- Binary index. Build a memory-mapped GFF3 index directly from GFF3 and genome FASTA with `helixblast-index`. The index adds no decoded full-index heap; the operating system pages in only the regions a query touches. JSON indexes remain supported and are decoded once, then cached by path. A `.bin` file next to a configured JSON path is detected automatically.
- Browser-local results. BLAST results are stored in the browser's IndexedDB for 24 hours. The job list and results live only in the browser; the server keeps a result until SSE delivery or the first `GET` request, then releases it.

## Quick start

Prerequisites:

- Go 1.26.3 or later
- Node.js 20 or later
- NCBI BLAST+ 2.17.0 or later

```bash
# Copy the sample configuration and add your databases.
cp config.yaml.example config.yaml
cp databases.yaml.example databases.yaml

# Build the server and the index tools.
make build

# Build a runtime index directly from GFF3 and genome FASTA.
./helixblast-index --gff3 annotations.gff3 --fasta genome.fa --out refseq.index.bin

# Point databases.yaml transcript.index_path at refseq.index.bin, then start the server.
./helixblast --config config.yaml
```

The server listens on port 8080 by default. Open `http://localhost:8080` after it starts.

## CLI tools

| Binary | Source | Purpose |
|---|---|---|
| `helixblast` | `cmd/server` | HTTP server with the REST API, SSE, and embedded frontend |
| `helixblast-index` | `cmd/indexer` | Build a binary index directly from GFF3 and genome FASTA. Use `--json` to also write an intermediate JSON index. |
| `helixblast-prepare` | `cmd/prepare` | Convert an existing JSON index to the binary format. Use this when the GFF3 source is no longer available. |
| `verify` | `cmd/verify` | Build a temporary binary index from a JSON index and compare entries, families, coordinates, and FASTA offsets. Build it with `make build-verify`. |

## Documentation

| Document | Content |
|---|---|
| [Configuration](configuration.md) | `config.yaml`, `databases.yaml`, resource detection, and hot reload |
| [API reference](api.md) | Endpoints, request and response fields, job lifecycle, and error codes |
| [Transcript lookup](transcript-lookup.md) | GFF3 index, local and Worker sequence extraction, and spatial search |
| [Architecture](architecture.md) | Binary index format, concurrency, SSE, frontend routes, and design system |
| [Changelog](CHANGELOG.md) | Notable changes by date |

## License

All code is licensed under the MIT License. The documentation, including this README, is licensed under CC BY-SA 4.0.
