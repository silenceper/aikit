BINARY_NAME := aikit
BIN_DIR := bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -s -w \
	-X github.com/silenceper/aikit/cmd.version=$(VERSION) \
	-X github.com/silenceper/aikit/cmd.commit=$(COMMIT) \
	-X github.com/silenceper/aikit/cmd.date=$(DATE)

.PHONY: build install clean test test-e2e run check check-docs check-format check-mod check-build check-test check-vet check-cross

build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) .

install:
	go install -ldflags "$(LDFLAGS)" .

clean:
	rm -rf $(BIN_DIR)

test:
	go test -race ./...

test-e2e: build
	bash scripts/test-e2e.sh

check-docs:
	bash scripts/check-docs-test.sh
	bash scripts/check-docs.sh

check-format:
	@files="$$(gofmt -l $$(git ls-files '*.go'))"; if [ -n "$$files" ]; then echo "Go files need formatting:"; echo "$$files"; exit 1; fi

check-mod:
	go mod tidy -diff

check-build:
	go build ./...

check-test:
	go test -race ./...

check-vet:
	go vet ./...

check-cross:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./...
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./...
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build ./...
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build ./...

check: check-docs check-format check-mod check-build check-test check-vet check-cross

run: build
	./$(BIN_DIR)/$(BINARY_NAME) $(ARGS)
