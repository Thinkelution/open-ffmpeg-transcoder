.PHONY: build run test clean docker docker-gpu docker-up docker-up-gpu lint

BINARY_NAME=transcoder
BUILD_DIR=bin
GO=go

# Build
build:
	$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/transcoder

build-linux:
	GOOS=linux GOARCH=amd64 $(GO) build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/transcoder

# Run
run: build
	./$(BUILD_DIR)/$(BINARY_NAME) all

run-api: build
	./$(BUILD_DIR)/$(BINARY_NAME) serve

run-worker: build
	./$(BUILD_DIR)/$(BINARY_NAME) worker

# Test
test:
	$(GO) test -v ./...

test-coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# Lint
lint:
	golangci-lint run ./...

# Clean
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Docker
docker:
	docker build -f docker/Dockerfile -t open-ffmpeg-transcoder .

docker-gpu:
	docker build -f docker/Dockerfile.gpu -t open-ffmpeg-transcoder:gpu .

docker-up:
	docker compose -f docker/docker-compose.yml up -d

docker-down:
	docker compose -f docker/docker-compose.yml down

docker-up-gpu:
	docker compose -f docker/docker-compose.yml -f docker/docker-compose.gpu.yml up -d

docker-logs:
	docker compose -f docker/docker-compose.yml logs -f transcoder

# Development helpers
dev-deps:
	docker compose -f docker/docker-compose.yml up -d redis postgres

migrate:
	$(GO) run ./cmd/transcoder migrate
