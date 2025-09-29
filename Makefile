.PHONY: build build-all install clean run test release

VERSION ?= dev

# Build for current platform
build:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o hours-mcp main.go

# Build for Apple Silicon specifically
build-arm64:
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o hours-mcp main.go

# Build for all platforms
build-all:
	./scripts/build-all.sh $(VERSION)

# Install to user's local bin
install: build-arm64
	mkdir -p ~/.local/bin
	cp hours-mcp ~/.local/bin/
	@echo "Installed to ~/.local/bin/hours-mcp"
	@echo "Add the following to your Claude Desktop config:"
	@echo '  "hours": {'
	@echo '    "command": "'$$HOME'/.local/bin/hours-mcp",'
	@echo '    "args": [],'
	@echo '    "env": {}'
	@echo '  }'

# Create a local release
release: build-all
	@echo "Creating release archive..."
	tar -czf hours-mcp-$(VERSION).tar.gz -C dist .
	@echo "Release archive created: hours-mcp-$(VERSION).tar.gz"

# Clean build artifacts
clean:
	rm -f hours-mcp hours-mcp-*.tar.gz
	rm -rf dist/

# Run locally for testing
run:
	go run main.go

# Run tests
test:
	go test ./...

# Download dependencies
deps:
	go mod download
	go mod tidy