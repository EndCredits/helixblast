.PHONY: build build-frontend build-prepare build-index build-verify dev clean lint lint-go lint-frontend run

build: build-frontend build-prepare build-index
	go build -o helixblast ./cmd/server

build-frontend:
	cd web && npm ci && npm run build
	rm -rf embed/assets embed/index.html && cp -r web/dist/* embed/

build-prepare:
	go build -o helixblast-prepare ./cmd/prepare

build-index:
	go build -o helixblast-index ./cmd/indexer

build-verify:
	go build -o verify ./cmd/verify

dev:
	go run ./cmd/server --config config.yaml

clean:
	rm -f helixblast
	rm -rf embed/assets embed/index.html
	rm -rf web/dist/
	rm -rf data/

lint: lint-go lint-frontend

lint-go:
	golangci-lint run ./...

lint-frontend:
	cd web && npm run lint

run:
	go run ./cmd/server --config config.yaml
