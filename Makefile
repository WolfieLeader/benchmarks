.PHONY: benchmark \
	honojs fastapi express nestjs fastify chi gin fiber \
	go-dev-% node-dev-% \
	build build-all build-honojs build-fastapi build-go-% build-node-% \
	install-honojs install-fastapi install-express install-nestjs install-fastify \
	install-chi install-gin install-fiber install-go-% install-node-% install-all \
	clean build-images clean-images

BUN_DIR := bun-honojs
PY_DIR := python-fastapi
NODE_DIR_PREFIX := node
GO_DIR_PREFIX := go

benchmark:
	cd benchmark && GOEXPERIMENT=jsonv2 go run ./cmd/main.go

# Development servers
dev: honojs fastapi express nestjs fastify chi gin fiber

honojs:
	cd $(BUN_DIR) && bun run dev

fastapi:
	cd $(PY_DIR) && uv run uvicorn src.main:app --reload --host 0.0.0.0 --port 4001

node-dev-%:
	cd $(NODE_DIR_PREFIX)-$* && pnpm dev

express: node-dev-express

nestjs: node-dev-nestjs

fastify: node-dev-fastify

go-dev-%:
	cd $(GO_DIR_PREFIX)-$* && air

chi: go-dev-chi

fiber: go-dev-fiber

gin: go-dev-gin

# Installation
install: install-all

install-honojs:
	@echo "Installing Hono.js dependencies..."
	cd $(BUN_DIR) && bun install

install-fastapi:
	@echo "Installing FastAPI dependencies..."
	cd $(PY_DIR) && uv sync

install-node-%:
	@echo "Installing Node $* dependencies..."
	cd $(NODE_DIR_PREFIX)-$* && pnpm install && pnpm update --latest

install-express: install-node-express

install-nestjs: install-node-nestjs

install-fastify: install-node-fastify

install-go-%:
	@echo "Installing Go $* dependencies..."
	cd $(GO_DIR_PREFIX)-$* && go mod tidy

install-chi: install-go-chi

install-fiber: install-go-fiber

install-gin: install-go-gin

install-all: install-honojs install-fastapi install-express install-nestjs install-fastify install-chi install-gin install-fiber
	@echo "All dependencies installed!"

# Build
build: build-all

build-all: build-honojs build-fastapi build-node-express build-node-nestjs build-node-fastify build-go-chi build-go-gin build-go-fiber

build-honojs:
	@echo "No build step for bun-honojs yet."

build-fastapi:
	@echo "No build step for python-fastapi yet."

build-node-%:
	cd $(NODE_DIR_PREFIX)-$* && pnpm build

build-go-%:
	cd $(GO_DIR_PREFIX)-$* && go build ./cmd

# Cleanup
clean:
	@echo "Cleaning build artifacts..."
	rm -rf \
		python-fastapi/.venv \
		python-fastapi/__pycache__ \
		python-fastapi/src/__pycache__ \
		bun-honojs/node_modules \
		node-express/node_modules \
		node-nestjs/node_modules \
		node-fastify/node_modules
	@echo "Clean complete!"

build-images:
	docker build -t bun-honojs-image ./bun-honojs
	docker build -t node-express-image ./node-express
	docker build -t node-nestjs-image ./node-nestjs
	docker build -t node-fastify-image ./node-fastify
	docker build -t python-fastapi-image ./python-fastapi
	docker build -t go-chi-image ./go-chi
	docker build -t go-fiber-image ./go-fiber
	docker build -t go-gin-image ./go-gin

clean-images:
	docker rmi bun-honojs-image
	docker rmi node-express-image
	docker rmi node-nestjs-image
	docker rmi node-fastify-image
	docker rmi python-fastapi-image
	docker rmi go-chi-image
	docker rmi go-fiber-image
	docker rmi go-gin-image
