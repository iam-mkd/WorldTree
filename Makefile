VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -s -w -X github.com/iam-mkd/WorldTree/internal/cli.version=$(VERSION) -X github.com/iam-mkd/WorldTree/internal/cli.commit=$(COMMIT)

.PHONY: build test lint

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/worldtree ./cmd/worldtree

test:
	go test ./...

lint:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && echo "gofmt needed" && exit 1)
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
