# ==============================================================================
# Benchmarks Makefile
# ==============================================================================

.PHONY: benchmark \
	honojs elysia oak express nestjs fastify chi gin fiber fastapi \
	install-honojs install-elysia install-oak install-express install-nestjs install-fastify \
	install-chi install-gin install-fiber install-fastapi install \
	update-honojs update-elysia update-oak update-express update-nestjs update-fastify \
	update-chi update-gin update-fiber update-fastapi update \
	clean images clean-images fmt lint tools install-root-tools update-root-tools

# ==============================================================================
# Benchmark Runner
# ==============================================================================

benchmark:
	cd benchmarks && go run ./cmd/main.go

# ==============================================================================
# Development Servers
# ==============================================================================

# --- Bun ---
honojs:
	cd http-servers/typescript/bun-honojs && bun run dev

elysia:
	cd http-servers/typescript/bun-elysia && bun run dev

# --- Deno ---
oak:
	cd http-servers/typescript/deno-oak && deno task dev

# --- Node.js ---
express:
	cd http-servers/typescript/node-express && pnpm run dev

nestjs:
	cd http-servers/typescript/node-nestjs && pnpm run dev

fastify:
	cd http-servers/typescript/node-fastify && pnpm run dev

# --- Go ---
chi:
	cd http-servers/go/chi && air

gin:
	cd http-servers/go/gin && air

fiber:
	cd http-servers/go/fiber && air

# --- Python ---
fastapi:
	cd http-servers/python/fastapi && uv run python -m src.main

# ==============================================================================
# Install Dependencies
# ==============================================================================

# --- Bun ---
install-honojs:
	cd http-servers/typescript/bun-honojs && bun install

install-elysia:
	cd http-servers/typescript/bun-elysia && bun install

# --- Deno ---
install-oak:
	cd http-servers/typescript/deno-oak && deno install

# --- Node.js ---
install-express:
	cd http-servers/typescript/node-express && pnpm install

install-nestjs:
	cd http-servers/typescript/node-nestjs && pnpm install

install-fastify:
	cd http-servers/typescript/node-fastify && pnpm install

# --- Go ---
install-chi:
	cd http-servers/go/chi && go mod tidy

install-gin:
	cd http-servers/go/gin && go mod tidy

install-fiber:
	cd http-servers/go/fiber && go mod tidy

# --- Python ---
install-fastapi:
	cd http-servers/python/fastapi && uv sync

# --- All ---
install-root-tools:
	pnpm install

install: install-root-tools \
	install-honojs \
	install-elysia \
	install-oak \
	install-express \
	install-nestjs \
	install-fastify \
	install-chi \
	install-gin \
	install-fiber \
	install-fastapi

# ==============================================================================
# Update Dependencies
# ==============================================================================

# --- Bun ---
update-honojs:
	cd http-servers/typescript/bun-honojs && bun update --latest

update-elysia:
	cd http-servers/typescript/bun-elysia && bun update --latest

# --- Deno ---
update-oak:
	cd http-servers/typescript/deno-oak && deno outdated --update

# --- Node.js ---
update-express:
	cd http-servers/typescript/node-express && pnpm update --latest

update-nestjs:
	cd http-servers/typescript/node-nestjs && pnpm update --latest

update-fastify:
	cd http-servers/typescript/node-fastify && pnpm update --latest

# --- Go ---
update-chi:
	cd http-servers/go/chi && go get -u ./... && go mod tidy

update-gin:
	cd http-servers/go/gin && go get -u ./... && go mod tidy

update-fiber:
	cd http-servers/go/fiber && go get -u ./... && go mod tidy

# --- Python ---
update-fastapi:
	cd http-servers/python/fastapi && uv sync --upgrade

# --- All ---
update-root-tools:
	pnpm update --latest

update: update-root-tools \
	update-honojs \
	update-elysia \
	update-oak \
	update-express \
	update-nestjs \
	update-fastify \
	update-chi \
	update-gin \
	update-fiber \
	update-fastapi

# ==============================================================================
# Docker
# ==============================================================================

images:
	docker build -t bun-honojs ./http-servers/typescript/bun-honojs
	docker build -t bun-elysia ./http-servers/typescript/bun-elysia
	docker build -t deno-oak ./http-servers/typescript/deno-oak
	docker build -t node-express ./http-servers/typescript/node-express
	docker build -t node-nestjs ./http-servers/typescript/node-nestjs
	docker build -t node-fastify ./http-servers/typescript/node-fastify
	docker build -t python-fastapi ./http-servers/python/fastapi
	docker build -t go-chi ./http-servers/go/chi
	docker build -t go-gin ./http-servers/go/gin
	docker build -t go-fiber ./http-servers/go/fiber

clean-images:
	docker rmi bun-honojs bun-elysia deno-oak \
		node-express node-nestjs node-fastify \
		python-fastapi go-chi go-gin go-fiber

# ==============================================================================
# Cleanup
# ==============================================================================

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf \
		http-servers/python/fastapi/.venv \
		http-servers/python/fastapi/__pycache__ \
		http-servers/python/fastapi/src/__pycache__ \
		http-servers/typescript/bun-honojs/node_modules \
		http-servers/typescript/bun-elysia/node_modules \
		http-servers/typescript/node-express/node_modules \
		http-servers/typescript/node-nestjs/node_modules \
		http-servers/typescript/node-fastify/node_modules \
		http-servers/typescript/deno-oak/node_modules
	@echo "Clean complete!"

# ==============================================================================
# Format & Lint (All)
# ==============================================================================

fmt:
	cd benchmarks && golangci-lint fmt ./...
	cd http-servers/go/chi && golangci-lint fmt ./...
	cd http-servers/go/gin && golangci-lint fmt ./...
	cd http-servers/go/fiber && golangci-lint fmt ./...
	cd http-servers/typescript/node-express && pnpm run format
	cd http-servers/typescript/node-fastify && pnpm run format
	cd http-servers/typescript/node-nestjs && pnpm run format
	cd http-servers/typescript/bun-honojs && bun run format
	cd http-servers/typescript/bun-elysia && bun run format
	cd http-servers/typescript/deno-oak && deno task format
	cd http-servers/python/fastapi && uv run ruff format .
	pnpm run format:md

lint:
	cd benchmarks && golangci-lint run ./...
	cd http-servers/go/chi && golangci-lint run ./...
	cd http-servers/go/gin && golangci-lint run ./...
	cd http-servers/go/fiber && golangci-lint run ./...
	cd http-servers/typescript/node-express && pnpm run lint
	cd http-servers/typescript/node-fastify && pnpm run lint
	cd http-servers/typescript/node-nestjs && pnpm run lint
	cd http-servers/typescript/bun-honojs && bun run lint
	cd http-servers/typescript/bun-elysia && bun run lint
	cd http-servers/typescript/deno-oak && deno task lint
	cd http-servers/python/fastapi && uv run ruff check .
	pnpm run lint:md
