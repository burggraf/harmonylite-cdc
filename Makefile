# Makefile for HarmonyLite

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
GIT_TAG ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "none")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GO_VERSION ?= $(shell go version | awk '{print $$3}')
PLATFORM ?= $(shell go env GOOS)-$(shell go env GOARCH)

# Linker flags
LDFLAGS = -s -w \
	-X github.com/wongfei2009/harmonylite/version.version=$(VERSION) \
	-X github.com/wongfei2009/harmonylite/version.gitCommit=$(GIT_COMMIT) \
	-X github.com/wongfei2009/harmonylite/version.gitTag=$(GIT_TAG) \
	-X github.com/wongfei2009/harmonylite/version.buildDate=$(BUILD_DATE) \
	-X github.com/wongfei2009/harmonylite/version.goVersion=$(GO_VERSION) \
	-X github.com/wongfei2009/harmonylite/version.platform=$(PLATFORM)

# Target for building the binary
.PHONY: build
build:
	CGO_ENABLED=1 CGO_CFLAGS="-Wno-typedef-redefinition -Wno-nullability-completeness" go build -ldflags "$(LDFLAGS)" -o harmonylite .

# Target for building statically linked binary
.PHONY: build-static
build-static:
	CGO_ENABLED=1 CGO_CFLAGS="-Wno-typedef-redefinition -Wno-nullability-completeness" go build -ldflags "$(LDFLAGS) -extldflags '-static'" -o harmonylite .

# Target for building for specific platforms
.PHONY: build-linux-amd64
build-linux-amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 CGO_CFLAGS="-Wno-typedef-redefinition -Wno-nullability-completeness" go build -ldflags "$(LDFLAGS)" -o harmonylite .

# Target for running tests
.PHONY: test
test:
	go test -v ./...

# Target for running unit tests
.PHONY: test-unit
test-unit:
	go test -v ./tests/unit/...

# Target for running integration tests
.PHONY: test-integration
test-integration:
	go test -v ./tests/integration/...

# Target for running E2E tests with Ginkgo
.PHONY: test-e2e
test-e2e:
	go run github.com/onsi/ginkgo/v2/ginkgo -v ./tests/e2e/...

# Target for running all tests with coverage
.PHONY: test-coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Target for installing the binary
.PHONY: install
install:
	CGO_ENABLED=1 go install -ldflags "$(LDFLAGS)" .

# Target for cleaning build artifacts
.PHONY: clean
clean:
	rm -f harmonylite coverage.out coverage.html
