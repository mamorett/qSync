VERSION ?= $(shell git describe --tags --always --dirty || echo dev)
GOFLAGS ?= -trimpath
LDFLAGS ?= -X main.version=$(VERSION)
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

.PHONY: build

# Build the binary
build:
	@echo "Building qsync version $(VERSION)"
	@mkdir -p bin
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o bin/qsync ./cmd/qsync
	@echo "Binary created: bin/qsync"

# Install to GOPATH
install:
	@echo "Installing qsync version $(VERSION)"
	CGO_ENABLED=0 go install $(GOFLAGS) -ldflags="$(LDFLAGS)" ./cmd/qsync
	@echo "Installed to $(shell go env GOPATH)/bin/qsync"

# Run tests with race detector
.PHONY: test
test:
	@echo "Running tests with race detector..."
	go test -race ./...

# Run short tests
test-short:
	@echo "Running short tests..."
	go test -short ./...

# Run go vet
.PHONY: vet
vet:
	@echo "Running go vet..."
	go vet ./...
	@echo "Checking code style..."
	@gofmt -l . > fmt.out
	@if [ -s fmt.out ]; then \
		echo "Files with formatting problems found:"; \
		cat fmt.out; \
		rm -f fmt.out; \
		false; \
	else \
		echo "✓ All files pass gofmt"; \
		rm -f fmt.out; \
	fi

# Build all GOOS/GOARCH combinations
.PHONY: release
release:
	@echo "Building release binaries for all platforms..."
	mkdir -p dist
	for os in linux darwin; do \
		for arch in amd64 arm64; do \
			GOOS=$$os GOARCH=$$arch $(MAKE) build version GOOS=$$os GOARCH=$$arch; \
			mv bin/qsync dist/qsync-$$os-$$arch || exit 1; \
			done \
		done
	@echo "Release binaries created in dist/ directory"
	@echo "Generating checksums..."
	cd dist && sha256sum qsync-* > qsync-checksums.txt

# Build with version output only
version-bin:
	@echo "qsync $(VERSION)"

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin dist fmt.out

# Show current version
.PHONY: version
version:
	@echo "Version: $(VERSION)"

# Show Go version
.PHONY: go-version
go-version:
	@go version

# Run all quality checks
.PHONY: check
check: vet test

# Default target
.PHONY: default
default: build