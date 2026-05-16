# HelixBLAST

Light, modern BLAST web service — single binary, zero external dependencies.

- **BLAST Search**: Web UI for NCBI BLAST+, with job queue, SSE streaming, alignment viewer
- **Transcript Lookup**: GFF3-based gene/transcript/CDS coordinate resolution and sequence extraction, backed by local FASTA or Cloudflare Worker + R2

## Quickstart

```bash
# Build
make build

# Configure
cp config.yaml.example config.yaml
# Edit config.yaml and databases.yaml

# Run
./helixblast --config config.yaml
```

## Documentation

| Document | Content |
|----------|---------|
| [Configuration](configuration.md) | `config.yaml`, `databases.yaml`, resource auto-detection, rationale |
| [API Reference](api.md) | All endpoints, request/response schemas |
| [Transcript Lookup](transcript-lookup.md) | GFF3 index, local vs Worker, region extraction, seeking |
| [Architecture](architecture.md) | Design decisions, data flow, concurrency model, job lifecycle |
