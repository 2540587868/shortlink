.PHONY: build test lint clean docker

BINARY=shortlink
CMD=./cmd/shortlink
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_TIME=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.buildTime=$(BUILD_TIME)"

build:
	go build $(LDFLAGS) -o $(BINARY) $(CMD)

test:
	go test ./... -count=1 -timeout 120s -race

test-cover:
	go test ./... -count=1 -timeout 120s -cover -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

lint:
	golangci-lint run ./... --timeout 5m

fmt:
	gofmt -w -s .

clean:
	rm -f $(BINARY) shortlink.exe coverage.out

docker:
	docker build -t shortlink:$(VERSION) -t shortlink:latest .

build-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)-linux-amd64 $(CMD)

build-arm:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)-linux-arm64 $(CMD)

release: build-linux build-arm
	@echo "Release binaries ready:"
	@ls -la $(BINARY)-linux-*