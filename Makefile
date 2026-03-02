.PHONY: build run test lint clean

BINARY_NAME=easyrss
BUILD_DIR=./bin

build:
	CGO_ENABLED=0 go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/server

run:
	go run ./cmd/server --config configs/config.yaml

test:
	go test ./... -v

lint:
	golangci-lint run

clean:
	rm -rf $(BUILD_DIR)

tidy:
	go mod tidy
