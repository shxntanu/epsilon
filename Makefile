BINARY_NAME := epsilon
BIN_DIR := bin
BINARY_PATH := $(BIN_DIR)/$(BINARY_NAME)
CMD_PACKAGE := ./cmd/epsilon-cli

.PHONY: build clean install-path

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BINARY_PATH) $(CMD_PACKAGE)

clean:
	rm -f $(BINARY_PATH)

install-path:
	./scripts/add-epsilon-to-path.sh
