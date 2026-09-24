set shell := ["bash", "-c"]

default:
    @just --list

setup:
    go mod download
    npm --prefix frontend ci

dev:
    trap 'kill 0' EXIT; go run . & npm --prefix frontend run dev & wait
dev-api:
    go run .
dev-web:
    npm --prefix frontend run dev

build: build-web
    go build -o bin/hyl .
build-web:
    npm --prefix frontend run build
run:
    go run .

test: build-web
    go test ./...
vet: build-web
    go vet ./...
fmt:
    gofmt -w .

generate: sqlc tygo
sqlc:
    sqlc generate
tygo:
    go tool tygo generate

db := env('HYL_DB_PATH', 'data/hyl.db')
migrate-up:
    goose -dir internal/db/migrations sqlite3 {{ db }} up
migrate-down:
    goose -dir internal/db/migrations sqlite3 {{ db }} down
migrate-status:
    goose -dir internal/db/migrations sqlite3 {{ db }} status
migrate-create name:
    goose -dir internal/db/migrations create -s {{ name }} sql

# protomaps prunes its old dated builds, so pass a newer <date>.pmtiles URL if the default has rotted.
tiles bbox maxzoom src='https://build.protomaps.com/20260924.pmtiles':
    mkdir -p data/tiles
    go run github.com/protomaps/go-pmtiles@v1.31.2 extract \
      --bbox={{ bbox }} --maxzoom={{ maxzoom }} --download-threads=4 \
      {{ src }} data/tiles/region.pmtiles
mailpit:
    docker run --rm -p 1025:1025 -p 8025:8025 axllent/mailpit
