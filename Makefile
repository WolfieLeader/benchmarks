.PHONY: honojs chi fiber gin fastapi all stop install-fastapi install-honojs install-chi install-all clean install-fiber install-gin build-images clean-images

# Development servers
honojs:
	cd bun-honojs && bun run dev

chi:
	cd go-chi && air

fiber:
	cd go-fiber && air

gin:
	cd go-gin && air

fastapi:
	@echo "Started development server: http://localhost:4000"
	@echo ""
	cd python-fastapi && uv run uvicorn src.main:app --reload --host 0.0.0.0 --port 4000


# Installation
install-honojs:
	@echo "Installing Hono.js dependencies..."
	cd bun-honojs && bun install

install-chi:
	@echo "Installing Go-Chi dependencies..."
	cd go-chi && go mod tidy

install-fiber:
	@echo "Installing Go-Fiber dependencies..."
	cd go-fiber && go mod tidy

install-gin:
	@echo "Installing Go-Gin dependencies..."
	cd go-gin && go mod tidy

install-fastapi:
	@echo "Installing FastAPI dependencies..."
	cd python-fastapi && uv sync

install-all: install-honojs install-chi install-fiber install-gin install-fastapi
	@echo "All dependencies installed!"

# Cleanup
clean:
	@echo "Cleaning build artifacts..."
	rm -rf python-fastapi/.venv
	rm -rf python-fastapi/__pycache__
	rm -rf python-fastapi/src/__pycache__
	rm -rf bun-honojs/node_modules
	@echo "Clean complete!"

build-images:
	docker build -t go-chi-image ./go-chi
	docker build -t go-fiber-image ./go-fiber
	docker build -t go-gin-image ./go-gin

clean-images:
	docker rmi go-chi-image
	docker rmi go-fiber-image
	docker rmi go-gin-image