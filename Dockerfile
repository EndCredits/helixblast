# Stage 1: Build frontend
FROM node:20-alpine AS frontend
WORKDIR /src
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/dist embed/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /helixblast ./cmd/server

# Stage 3: Runtime
FROM alpine:3.21
RUN apk add --no-cache ncbi-blast+ ca-certificates tzdata
COPY --from=builder /helixblast /usr/local/bin/helixblast
COPY config.yaml /etc/helixblast/config.yaml
COPY databases.yaml /etc/helixblast/databases.yaml
EXPOSE 8080
ENTRYPOINT ["helixblast", "--config", "/etc/helixblast/config.yaml"]
