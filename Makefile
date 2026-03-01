BINARY_NAME := aikit
BIN_DIR := bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -s -w \
	-X github.com/silenceper/aikit/cmd.version=$(VERSION) \
	-X github.com/silenceper/aikit/cmd.commit=$(COMMIT) \
	-X github.com/silenceper/aikit/cmd.date=$(DATE)

.PHONY: build install clean test test-e2e run

build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) .

install:
	go install -ldflags "$(LDFLAGS)" .

clean:
	rm -rf $(BIN_DIR)

test:
	go test ./...

test-e2e: build
	bash scripts/test-e2e.sh

run: build
	./$(BIN_DIR)/$(BINARY_NAME) $(ARGS)
