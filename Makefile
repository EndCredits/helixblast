.PHONY: build build-frontend dev clean lint lint-go lint-frontend run

build: build-frontend
	go build -o helixblast ./cmd/server

build-frontend:
	cd web && npm ci && npm run build
	rm -rf embed/assets embed/index.html && cp -r web/dist/* embed/

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
