BINARY_NAME := aikit
BIN_DIR := bin

.PHONY: build install clean test run

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY_NAME) .

install: build
	go install .

clean:
	rm -rf $(BIN_DIR)

test:
	go test ./...

run: build
	./$(BIN_DIR)/$(BINARY_NAME) $(ARGS)
