# Stage 1: Build frontend
FROM node:20-alpine AS frontend
WORKDIR /src
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.26.3-trixie AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN apt update -y && apt install -y git
RUN go mod download
COPY . .
COPY --from=frontend /src/dist embed/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /helixblast ./cmd/server

# Stage 3: Runtime
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    tzdata \
    libgomp1 \
    && rm -rf /var/lib/apt/lists/*

ENV BLAST_VERSION=2.17.0
RUN curl -fsSL "https://ftp.ncbi.nlm.nih.gov/blast/executables/blast+/LATEST/ncbi-blast-${BLAST_VERSION}+-x64-linux.tar.gz" \
    | tar xz -C /usr/local/bin --strip-components=2 --wildcards '*/bin/*'

COPY --from=builder /helixblast /usr/local/bin/helixblast
COPY config.yaml /etc/helixblast/config.yaml
COPY databases.yaml /etc/helixblast/databases.yaml

EXPOSE 8080
ENTRYPOINT ["helixblast", "--config", "/etc/helixblast/config.yaml"]
