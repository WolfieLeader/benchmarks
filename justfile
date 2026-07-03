@_default:
    just --list

servers_dir := "servers"

# Run benchmark suite (use --help for flags)
benchmark *args:
    cd benchmark && GOTOOLCHAIN=go1.27rc1 go run ./cmd/main.go {{args}}

alias benchmarks := benchmark

# Start a dev server (ts-bun-honojs, ts-bun-elysia, ts-deno-oak, ts-express, ts-nestjs, ts-fastify, go-chi, go-gin, go-fiber, py-fastapi)
[group('dev')]
dev server:
    node scripts/dev.mts {{server}}

# Install dependencies for a target (or 'all')
[group('deps')]
install target='all':
    node scripts/install.mts {{target}}

# Update dependencies for a target (or 'all') — pin-aware
[group('deps')]
update target='all':
    node scripts/update.mts {{target}}

# Start database stack
[group('docker')]
db-up:
    docker compose -f infra/compose/databases.yml up -d

# Stop database stack
[group('docker')]
db-down:
    docker compose -f infra/compose/databases.yml down -v

# Start Grafana/InfluxDB stack
[group('docker')]
grafana-up:
    docker compose -f infra/compose/grafana.yml down -v
    docker compose -f infra/compose/grafana.yml up -d
    @echo "Grafana: http://localhost:3000 (admin/123456)"

# Stop Grafana/InfluxDB stack
[group('docker')]
grafana-down:
    docker compose -f infra/compose/grafana.yml down -v

# View Grafana logs
[group('docker')]
grafana-logs:
    docker compose -f infra/compose/grafana.yml logs -f

# Build Docker image(s) for an entry (or 'all')
[group('docker')]
images entry='all':
    node scripts/images.mts {{entry}}

# Remove all Docker images (best effort)
[group('docker')]
remove-images:
    -docker rmi bench/ts-bun-honojs bench/ts-bun-elysia bench/ts-deno-oak bench/ts-express bench/ts-nestjs bench/ts-fastify bench/py-fastapi bench/go-chi bench/go-gin bench/go-fiber

# Remove build artifacts and node_modules
clean:
    #!/usr/bin/env bash
    set -euo pipefail
    if [[ ! -d "{{servers_dir}}" ]]; then
        echo "Refusing to clean: expected repo layout not found" && exit 1
    fi
    echo "Cleaning build artifacts..."
    rm -rf \
        "{{servers_dir}}/py-fastapi/.venv" \
        "{{servers_dir}}/py-fastapi/__pycache__" \
        "{{servers_dir}}/py-fastapi/src/__pycache__" \
        "{{servers_dir}}/ts-bun-honojs/node_modules" \
        "{{servers_dir}}/ts-bun-elysia/node_modules" \
        "{{servers_dir}}/ts-express/node_modules" \
        "{{servers_dir}}/ts-nestjs/node_modules" \
        "{{servers_dir}}/ts-fastify/node_modules" \
        "{{servers_dir}}/ts-deno-oak/node_modules" \
        "{{servers_dir}}/go-chi/bin" \
        "{{servers_dir}}/go-chi/tmp" \
        "{{servers_dir}}/go-gin/bin" \
        "{{servers_dir}}/go-gin/tmp" \
        "{{servers_dir}}/go-fiber/bin" \
        "{{servers_dir}}/go-fiber/tmp"
    echo "Clean complete!"

# Type check a target (or 'all')
[group('verify')]
typecheck target='all':
    node scripts/verify.mts {{target}} --only=typecheck

# Write-format code for a target (or 'all') — mutating
[group('verify')]
fmt target='all':
    node scripts/format.mts {{target}}

# Lint a target (or 'all')
[group('verify')]
lint target='all':
    node scripts/lint.mts {{target}}

# Contract conformance gate: build/run a server in a container, run the contract, tear down (or 'all')
[group('verify')]
contract entry='all':
    node scripts/contract.mts {{entry}}

alias conformance := contract

# Validate config.json + bench.json manifests against their schemas and cross-check for drift
[group('verify')]
check-config:
    node scripts/check-config.mts

# Non-mutating verification gate for a target (or 'all'): type/build check + format-check + lint
[group('verify')]
verify target='all':
    node scripts/verify.mts {{target}}

# Generate SQLC code for a Go framework (or 'all')
[group('codegen')]
sqlc target='all':
    #!/usr/bin/env bash
    set -euo pipefail
    gen_one() {
        case "$1" in
            go-chi)   (cd {{servers_dir}}/go-chi/internal/database/sqlc && sqlc generate) ;;
            go-gin)   (cd {{servers_dir}}/go-gin/internal/database/sqlc && sqlc generate) ;;
            go-fiber) (cd {{servers_dir}}/go-fiber/internal/database/sqlc && sqlc generate) ;;
            *) echo "Unknown target: $1" && exit 1 ;;
        esac
    }
    if [ "{{target}}" = "all" ]; then
        for t in go-chi go-gin go-fiber; do
            gen_one "$t"
        done
    else
        gen_one "{{target}}"
    fi
