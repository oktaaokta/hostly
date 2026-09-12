VITE_BASE ?= /
PORT ?= 8080
DB_PATH ?= hostly.db
BIN ?= bin/hostly

.PHONY: run demo test lint dist-check build frontend clean

dist-check:
	@test -d internal/webassets/dist || { echo "built SPA missing — run 'make frontend'"; exit 1; }

run demo test lint: dist-check

run:
	PORT=$(PORT) DB_PATH=$(DB_PATH) go run ./cmd/hostly

demo:
	PORT=$(PORT) DB_PATH=$(DB_PATH) go run ./cmd/hostly -seed

test:
	go test ./...

lint:
	golangci-lint run

frontend:
	cd web && VITE_BASE=$(VITE_BASE) npm ci --no-audit --no-fund && VITE_BASE=$(VITE_BASE) npm run build
	rm -rf internal/webassets/dist
	mkdir -p internal/webassets/dist
	cp -R web/dist/. internal/webassets/dist/

build: frontend
	CGO_ENABLED=0 go build -o $(BIN) ./cmd/hostly

clean:
	rm -rf $(BIN) internal/webassets/dist web/dist hostly.db