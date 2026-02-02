@_default:
    just --list

ts_dir := "http-servers/typescript"
go_dir := "http-servers/go"
py_dir := "http-servers/python"

# Run benchmark suite (use --help for flags)
benchmark *args:
    cd benchmarks && go run ./cmd/main.go {{args}}

alias benchmarks := benchmark

# Start a dev server (honojs, elysia, oak, express, nestjs, fastify, chi, gin, fiber, fastapi)
[group('dev')]
dev server:
    #!/usr/bin/env bash
    set -euo pipefail
    case "{{server}}" in
        honojs)   cd {{ts_dir}}/bun-honojs && bun run dev ;;
        elysia)   cd {{ts_dir}}/bun-elysia && bun run dev ;;
        oak)      cd {{ts_dir}}/deno-oak && deno task dev ;;
        express)  cd {{ts_dir}}/node-express && pnpm run dev ;;
        nestjs)   cd {{ts_dir}}/node-nestjs && pnpm run dev ;;
        fastify)  cd {{ts_dir}}/node-fastify && pnpm run dev ;;
        chi)      cd {{go_dir}}/chi && air ;;
        gin)      cd {{go_dir}}/gin && air ;;
        fiber)    cd {{go_dir}}/fiber && air ;;
        fastapi)  cd {{py_dir}}/fastapi && uv run python -m src.main ;;
        *) echo "Unknown server: {{server}}" && exit 1 ;;
    esac

# Install dependencies for a framework (or 'all')
[group('deps')]
install target='all':
    #!/usr/bin/env bash
    set -euo pipefail
    install_one() {
        case "$1" in
            honojs)    cd {{ts_dir}}/bun-honojs && bun install ;;
            elysia)    cd {{ts_dir}}/bun-elysia && bun install ;;
            oak)       cd {{ts_dir}}/deno-oak && deno install ;;
            express)   cd {{ts_dir}}/node-express && pnpm install ;;
            nestjs)    cd {{ts_dir}}/node-nestjs && pnpm install ;;
            fastify)   cd {{ts_dir}}/node-fastify && pnpm install ;;
            chi)       cd {{go_dir}}/chi && go mod tidy ;;
            gin)       cd {{go_dir}}/gin && go mod tidy ;;
            fiber)     cd {{go_dir}}/fiber && go mod tidy ;;
            fastapi)   cd {{py_dir}}/fastapi && uv sync ;;
            benchmark) cd benchmarks && go mod tidy ;;
            root)      pnpm install ;;
            *) echo "Unknown target: $1" && exit 1 ;;
        esac
    }
    if [ "{{target}}" = "all" ]; then
        for t in root honojs elysia oak express nestjs fastify chi gin fiber fastapi benchmark; do
            install_one "$t"
        done
    else
        install_one "{{target}}"
    fi

# Update dependencies for a framework (or 'all')
[group('deps')]
update target='all':
    #!/usr/bin/env bash
    set -euo pipefail
    update_one() {
        case "$1" in
            honojs)    cd {{ts_dir}}/bun-honojs && bun update --latest ;;
            elysia)    cd {{ts_dir}}/bun-elysia && bun update --latest ;;
            oak)       cd {{ts_dir}}/deno-oak && deno outdated --update ;;
            express)   cd {{ts_dir}}/node-express && pnpm update --latest ;;
            nestjs)    cd {{ts_dir}}/node-nestjs && pnpm update --latest ;;
            fastify)   cd {{ts_dir}}/node-fastify && pnpm update --latest ;;
            chi)       cd {{go_dir}}/chi && go get -u ./... && go mod tidy ;;
            gin)       cd {{go_dir}}/gin && go get -u ./... && go mod tidy ;;
            fiber)     cd {{go_dir}}/fiber && go get -u ./... && go mod tidy ;;
            fastapi)   cd {{py_dir}}/fastapi && uv sync --upgrade ;;
            benchmark) cd benchmarks && go get -u ./... && go mod tidy ;;
            root)      pnpm update --latest ;;
            *) echo "Unknown target: $1" && exit 1 ;;
        esac
    }
    if [ "{{target}}" = "all" ]; then
        for t in root honojs elysia oak express nestjs fastify chi gin fiber fastapi benchmark; do
            update_one "$t"
        done
    else
        update_one "{{target}}"
    fi

# Start database stack
[group('docker')]
db-up:
    docker compose -f infra/compose/databases.yml up -d

# Stop database stack
[group('docker')]
db-down:
    docker compose -f infra/compose/databases.yml down

# Start Grafana/InfluxDB stack
[group('docker')]
grafana-up:
    docker compose -f infra/compose/grafana.yml down
    docker compose -f infra/compose/grafana.yml up -d
    @echo "Grafana: http://localhost:3000 (admin/123456)"

# Stop Grafana/InfluxDB stack
[group('docker')]
grafana-down:
    docker compose -f infra/compose/grafana.yml down

# View Grafana logs
[group('docker')]
grafana-logs:
    docker compose -f infra/compose/grafana.yml logs -f

# Build all Docker images
[group('docker')]
images:
    docker build -t bun-honojs {{ts_dir}}/bun-honojs
    docker build -t bun-elysia {{ts_dir}}/bun-elysia
    docker build -t deno-oak {{ts_dir}}/deno-oak
    docker build -t node-express {{ts_dir}}/node-express
    docker build -t node-nestjs {{ts_dir}}/node-nestjs
    docker build -t node-fastify {{ts_dir}}/node-fastify
    docker build -t python-fastapi {{py_dir}}/fastapi
    docker build -t go-chi {{go_dir}}/chi
    docker build -t go-gin {{go_dir}}/gin
    docker build -t go-fiber {{go_dir}}/fiber

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
    #!/usr/bin/env bash
    set -euo pipefail
    check_one() {
        case "$1" in
            express)   cd {{ts_dir}}/node-express && pnpm run typecheck ;;
            fastify)   cd {{ts_dir}}/node-fastify && pnpm run typecheck ;;
            nestjs)    cd {{ts_dir}}/node-nestjs && pnpm run typecheck ;;
            honojs)    cd {{ts_dir}}/bun-honojs && bun run typecheck ;;
            elysia)    cd {{ts_dir}}/bun-elysia && bun run typecheck ;;
            oak)       cd {{ts_dir}}/deno-oak && deno task check ;;
            chi)       cd {{go_dir}}/chi && go build -o bin/server ./cmd/main.go ;;
            gin)       cd {{go_dir}}/gin && go build -o bin/server ./cmd/main.go ;;
            fiber)     cd {{go_dir}}/fiber && go build -o bin/server ./cmd/main.go ;;
            fastapi)   cd {{py_dir}}/fastapi && uv run pyright src ;;
            benchmark) cd benchmarks && go build -o bin/benchmark ./cmd/main.go ;;
            root)      echo "No typecheck for root" ;;
            *) echo "Unknown target: $1" && exit 1 ;;
        esac
    }
    if [ "{{target}}" = "all" ]; then
        for t in express fastify nestjs honojs elysia oak chi gin fiber fastapi benchmark; do
            check_one "$t"
        done
    else
        check_one "{{target}}"
    fi

# Format code for a target (or 'all')
[group('verify')]
fmt target='all':
    #!/usr/bin/env bash
    set -euo pipefail
    fmt_one() {
        case "$1" in
            express)   cd {{ts_dir}}/node-express && pnpm run format ;;
            fastify)   cd {{ts_dir}}/node-fastify && pnpm run format ;;
            nestjs)    cd {{ts_dir}}/node-nestjs && pnpm run format ;;
            honojs)    cd {{ts_dir}}/bun-honojs && bun run format ;;
            elysia)    cd {{ts_dir}}/bun-elysia && bun run format ;;
            oak)       cd {{ts_dir}}/deno-oak && deno task format ;;
            chi)       cd {{go_dir}}/chi && golangci-lint fmt ./... ;;
            gin)       cd {{go_dir}}/gin && golangci-lint fmt ./... ;;
            fiber)     cd {{go_dir}}/fiber && golangci-lint fmt ./... ;;
            fastapi)   cd {{py_dir}}/fastapi && uv run ruff format . ;;
            benchmark) cd benchmarks && golangci-lint fmt ./... ;;
            root)      pnpm run format:md ;;
            *) echo "Unknown target: $1" && exit 1 ;;
        esac
    }
    if [ "{{target}}" = "all" ]; then
        for t in express fastify nestjs honojs elysia oak chi gin fiber fastapi benchmark root; do
            fmt_one "$t"
        done
    else
        fmt_one "{{target}}"
    fi

# Lint code for a target (or 'all')
[group('verify')]
lint target='all':
    #!/usr/bin/env bash
    set -euo pipefail
    lint_one() {
        case "$1" in
            express)   cd {{ts_dir}}/node-express && pnpm run lint:fix ;;
            fastify)   cd {{ts_dir}}/node-fastify && pnpm run lint:fix ;;
            nestjs)    cd {{ts_dir}}/node-nestjs && pnpm run lint:fix ;;
            honojs)    cd {{ts_dir}}/bun-honojs && bun run lint:fix ;;
            elysia)    cd {{ts_dir}}/bun-elysia && bun run lint:fix ;;
            oak)       cd {{ts_dir}}/deno-oak && deno task lint:fix ;;
            chi)       cd {{go_dir}}/chi && golangci-lint run ./... ;;
            gin)       cd {{go_dir}}/gin && golangci-lint run ./... ;;
            fiber)     cd {{go_dir}}/fiber && golangci-lint run ./... ;;
            fastapi)   cd {{py_dir}}/fastapi && uv run ruff check . ;;
            benchmark) cd benchmarks && golangci-lint run ./... ;;
            root)      pnpm run lint:md ;;
            *) echo "Unknown target: $1" && exit 1 ;;
        esac
    }
    if [ "{{target}}" = "all" ]; then
        for t in express fastify nestjs honojs elysia oak chi gin fiber fastapi benchmark root; do
            lint_one "$t"
        done
    else
        lint_one "{{target}}"
    fi

# Run full verification for a target (or 'all'): typecheck -> fmt -> lint
[group('verify')]
verify target='all':
    just typecheck {{target}}
    just fmt {{target}}
    just lint {{target}}

# Generate SQLC code for a Go framework (or 'all')
[group('codegen')]
sqlc target='all':
    #!/usr/bin/env bash
    set -euo pipefail
    gen_one() {
        case "$1" in
            chi)   cd {{go_dir}}/chi/internal/database/sqlc && sqlc generate ;;
            gin)   cd {{go_dir}}/gin/internal/database/sqlc && sqlc generate ;;
            fiber) cd {{go_dir}}/fiber/internal/database/sqlc && sqlc generate ;;
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
