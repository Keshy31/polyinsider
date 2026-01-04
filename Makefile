.PHONY: build run clean test deps

# Binary output directory
BIN_DIR := bin
BINARY := $(BIN_DIR)/engine

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GORUN := $(GOCMD) run
GOTEST := $(GOCMD) test
GOMOD := $(GOCMD) mod

# Build the binary
build:
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) -o $(BINARY) ./cmd/engine

# Run the application
run: build
	./$(BINARY)

# Run without building binary
dev:
	$(GORUN) ./cmd/engine

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Run tests
test:
	$(GOTEST) -v ./...

# Clean build artifacts
clean:
	@rm -rf $(BIN_DIR)
	@rm -f coverage.out

# Create data directory
init:
	@mkdir -p data

# Help
help:
	@echo "Available targets:"
	@echo "  build  - Build the binary"
	@echo "  run    - Build and run the application"
	@echo "  dev    - Run without building binary"
	@echo "  deps   - Download and tidy dependencies"
	@echo "  test   - Run tests"
	@echo "  clean  - Remove build artifacts"
	@echo "  init   - Create data directory"

