.PHONY: honojs chi fastapi all stop install-fastapi install-honojs install-chi install-all clean

# Development servers
honojs:
	cd bun-honojs && bun run dev

chi:
	cd go-chi && air

fastapi:
	@echo "Started development server: http://localhost:4000"
	@echo ""
	cd python-fastapi && uv run uvicorn src.main:app --reload --host 0.0.0.0 --port 4000


all:
	@echo "Starting all servers..."
	@echo "FastAPI will be on port 4000"
	@echo "Press Ctrl+C to stop all servers"
	@make -j3 honojs chi fastapi

# Installation
install-honojs:
	@echo "Installing Hono.js dependencies..."
	cd bun-honojs && bun install

install-chi:
	@echo "Installing Go-Chi dependencies..."
	cd go-chi && go mod tidy

install-fastapi:
	@echo "Installing FastAPI dependencies..."
	cd python-fastapi && uv sync

install-all: install-honojs install-chi install-fastapi
	@echo "All dependencies installed!"

# Cleanup
clean:
	@echo "Cleaning build artifacts..."
	rm -rf python-fastapi/.venv
	rm -rf python-fastapi/__pycache__
	rm -rf python-fastapi/src/__pycache__
	rm -rf bun-honojs/node_modules
	@echo "Clean complete!"