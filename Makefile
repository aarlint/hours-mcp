.PHONY: build install clean run test

# Build for Apple Silicon
build:
	GOOS=darwin GOARCH=arm64 go build -o hours-mcp main.go

# Install to user's local bin
install: build
	mkdir -p ~/.local/bin
	cp hours-mcp ~/.local/bin/
	@echo "Installed to ~/.local/bin/hours-mcp"
	@echo "Add the following to your Claude Desktop config:"
	@echo '  "hours": {'
	@echo '    "command": "'$$HOME'/.local/bin/hours-mcp",'
	@echo '    "args": [],'
	@echo '    "env": {}'
	@echo '  }'

# Clean build artifacts
clean:
	rm -f hours-mcp

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