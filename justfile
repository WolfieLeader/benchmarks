@_default:
    just --list

ts_dir := "http-servers/typescript"
go_dir := "http-servers/go"
py_dir := "http-servers/python"

# Run benchmark suite (use --help for flags)
benchmark *args:
    cd benchmarks && GOTOOLCHAIN=go1.27rc1 go run ./cmd/main.go {{args}}

alias benchmarks := benchmark

# Start a dev server (honojs, elysia, oak, express, nestjs, fastify, chi, gin, fiber, fastapi)
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
    -docker rmi bun-honojs bun-elysia deno-oak node-express node-nestjs node-fastify python-fastapi go-chi go-gin go-fiber

# Remove build artifacts and node_modules
clean:
    #!/usr/bin/env bash
    set -euo pipefail
    if [[ ! -d "{{py_dir}}" || ! -d "{{ts_dir}}" || ! -d "{{go_dir}}" ]]; then
        echo "Refusing to clean: expected repo layout not found" && exit 1
    fi
    echo "Cleaning build artifacts..."
    rm -rf \
        "{{py_dir}}/fastapi/.venv" \
        "{{py_dir}}/fastapi/__pycache__" \
        "{{py_dir}}/fastapi/src/__pycache__" \
        "{{ts_dir}}/bun-honojs/node_modules" \
        "{{ts_dir}}/bun-elysia/node_modules" \
        "{{ts_dir}}/node-express/node_modules" \
        "{{ts_dir}}/node-nestjs/node_modules" \
        "{{ts_dir}}/node-fastify/node_modules" \
        "{{ts_dir}}/deno-oak/node_modules" \
        "{{go_dir}}/chi/bin" \
        "{{go_dir}}/chi/tmp" \
        "{{go_dir}}/gin/bin" \
        "{{go_dir}}/gin/tmp" \
        "{{go_dir}}/fiber/bin" \
        "{{go_dir}}/fiber/tmp"
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
            chi)   (cd {{go_dir}}/chi/internal/database/sqlc && sqlc generate) ;;
            gin)   (cd {{go_dir}}/gin/internal/database/sqlc && sqlc generate) ;;
            fiber) (cd {{go_dir}}/fiber/internal/database/sqlc && sqlc generate) ;;
            *) echo "Unknown target: $1" && exit 1 ;;
        esac
    }
    if [ "{{target}}" = "all" ]; then
        for t in chi gin fiber; do
            gen_one "$t"
        done
    else
        gen_one "{{target}}"
    fi
