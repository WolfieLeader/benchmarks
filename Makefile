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
	cd benchmark && go run ./cmd/main.go

# ==============================================================================
# Development Servers
# ==============================================================================

# --- Bun ---
honojs:
	cd bun-honojs && bun run dev

elysia:
	cd bun-elysia && bun run dev

# --- Deno ---
oak:
	cd deno-oak && deno task dev

# --- Node.js ---
express:
	cd node-express && pnpm run dev

nestjs:
	cd node-nestjs && pnpm run dev

fastify:
	cd node-fastify && pnpm run dev

# --- Go ---
chi:
	cd go-chi && air

gin:
	cd go-gin && air

fiber:
	cd go-fiber && air

# --- Python ---
fastapi:
	cd python-fastapi && uv run python -m src.main

# ==============================================================================
# Install Dependencies
# ==============================================================================

# --- Bun ---
install-honojs:
	cd bun-honojs && bun install

install-elysia:
	cd bun-elysia && bun install

# --- Deno ---
install-oak:
	cd deno-oak && deno install

# --- Node.js ---
install-express:
	cd node-express && pnpm install

install-nestjs:
	cd node-nestjs && pnpm install

install-fastify:
	cd node-fastify && pnpm install

# --- Go ---
install-chi:
	cd go-chi && go mod tidy

install-gin:
	cd go-gin && go mod tidy

install-fiber:
	cd go-fiber && go mod tidy

# --- Python ---
install-fastapi:
	cd python-fastapi && uv sync

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
	cd bun-honojs && bun update --latest

update-elysia:
	cd bun-elysia && bun update --latest

# --- Deno ---
update-oak:
	cd deno-oak && deno outdated --update

# --- Node.js ---
update-express:
	cd node-express && pnpm update --latest

update-nestjs:
	cd node-nestjs && pnpm update --latest

update-fastify:
	cd node-fastify && pnpm update --latest

# --- Go ---
update-chi:
	cd go-chi && go get -u ./... && go mod tidy

update-gin:
	cd go-gin && go get -u ./... && go mod tidy

update-fiber:
	cd go-fiber && go get -u ./... && go mod tidy

# --- Python ---
update-fastapi:
	cd python-fastapi && uv sync --upgrade

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
	docker build -t bun-honojs ./bun-honojs
	docker build -t bun-elysia ./bun-elysia
	docker build -t deno-oak ./deno-oak
	docker build -t node-express ./node-express
	docker build -t node-nestjs ./node-nestjs
	docker build -t node-fastify ./node-fastify
	docker build -t python-fastapi ./python-fastapi
	docker build -t go-chi ./go-chi
	docker build -t go-gin ./go-gin
	docker build -t go-fiber ./go-fiber

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
		python-fastapi/.venv \
		python-fastapi/__pycache__ \
		python-fastapi/src/__pycache__ \
		bun-honojs/node_modules \
		bun-elysia/node_modules \
		node-express/node_modules \
		node-nestjs/node_modules \
		node-fastify/node_modules \
		deno-oak/node_modules
	@echo "Clean complete!"

# ==============================================================================
# Format & Lint (All)
# ==============================================================================

fmt:
	cd benchmark && golangci-lint fmt ./...
	cd go-chi && golangci-lint fmt ./...
	cd go-gin && golangci-lint fmt ./...
	cd go-fiber && golangci-lint fmt ./...
	cd node-express && pnpm run format
	cd node-fastify && pnpm run format
	cd node-nestjs && pnpm run format
	cd bun-honojs && bun run format
	cd bun-elysia && bun run format
	cd deno-oak && deno task format
	cd python-fastapi && uv run ruff format .
	pnpm run format:md

lint:
	cd benchmark && golangci-lint run ./...
	cd go-chi && golangci-lint run ./...
	cd go-gin && golangci-lint run ./...
	cd go-fiber && golangci-lint run ./...
	cd node-express && pnpm run lint
	cd node-fastify && pnpm run lint
	cd node-nestjs && pnpm run lint
	cd bun-honojs && bun run lint
	cd bun-elysia && bun run lint
	cd deno-oak && deno task lint
	cd python-fastapi && uv run ruff check .
	pnpm run lint:md
