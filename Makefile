.PHONY: help dev-fastapi dev-honojs dev-all stop install-fastapi install-honojs install-all clean

# Default target - show help
help:
	@echo "==================================="
	@echo "Backend Benchmark Project Commands"
	@echo "==================================="
	@echo ""
	@echo "Development:"
	@echo "  make dev-fastapi    - Run FastAPI server (port 4000)"
	@echo "  make dev-honojs     - Run Hono.js server"
	@echo "  make dev-all        - Run all servers concurrently"
	@echo ""
	@echo "Installation:"
	@echo "  make install-fastapi  - Install FastAPI dependencies"
	@echo "  make install-honojs   - Install Hono.js dependencies"
	@echo "  make install-all      - Install all dependencies"
	@echo ""
	@echo "Cleanup:"
	@echo "  make clean          - Clean all build artifacts"
	@echo ""

# Development servers
dev-honojs:
	cd honojs && bun run dev

dev-chi:
	cd go-chi && GOEXPERIMENT=jsonv2 go run cmd/main.go

dev-fastapi:
	@echo "Started development server: http://localhost:4000"
	@echo ""
	cd fastapi && uv run uvicorn src.main:app --reload --host 0.0.0.0 --port 4000


dev-all:
	@echo "Starting all servers..."
	@echo "FastAPI will be on port 4000"
	@echo "Press Ctrl+C to stop all servers"
	@make -j2 dev-fastapi dev-honojs

# Installation
install-honojs:
	@echo "Installing Hono.js dependencies..."
	cd honojs && bun install

install-chi:
	@echo "Installing Go-Chi dependencies..."
	cd go-chi && go mod tidy

install-fastapi:
	@echo "Installing FastAPI dependencies..."
	cd fastapi && uv sync

install-all: install-honojs install-chi install-fastapi
	@echo "All dependencies installed!"

# Cleanup
clean:
	@echo "Cleaning build artifacts..."
	rm -rf fastapi/.venv
	rm -rf fastapi/__pycache__
	rm -rf fastapi/src/__pycache__
	rm -rf honojs/node_modules
	@echo "Clean complete!"