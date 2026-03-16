VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -s -w -X main.version=$(VERSION)

.PHONY: build install test lint clean

build:
	go build -ldflags '$(LDFLAGS)' -o bin/devctl ./cmd/devctl/

install:
	go install -ldflags '$(LDFLAGS)' ./cmd/devctl/

test:
	go test ./...

lint:
	golangci-lint run

clean:
	rm -rf bin/ dist/
