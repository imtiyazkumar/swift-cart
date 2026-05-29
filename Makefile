SHELL := /usr/bin/env bash

# Binary name
BINARY := swiftkart-api

# Go command
GO := go

# Build flags
LDFLAGS := -s -w

.PHONY: all build run test lint fmt tidy docker-build docker-up docker-down clean

all: build

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/api

run: build
	./$(BINARY)

test:
	$(GO) test ./... -cover ./... -count=1

lint:
	golangci-lint run ./...

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

docker-build:
	docker build -t $(BINARY):latest .

docker-up:
	docker compose up -d

docker-down:
	docker compose down

clean:
	rm -f $(BINARY)
