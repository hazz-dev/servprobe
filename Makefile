BINARY := svcmon
BIN_DIR := bin
CMD := ./cmd/svcmon

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "-X github.com/hazz-dev/svcmon/internal/version.Version=$(VERSION) \
                     -X github.com/hazz-dev/svcmon/internal/version.Commit=$(COMMIT) \
                     -X github.com/hazz-dev/svcmon/internal/version.Date=$(DATE)"

.PHONY: all fmt vet lint test build clean

all: fmt vet lint test build

fmt:
	gofmt -w .

vet:
	go vet ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found, skipping (run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)"; \
	fi

test:
	go test ./...

build:
	mkdir -p $(BIN_DIR)
	go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY) $(CMD)

clean:
	rm -rf $(BIN_DIR)
