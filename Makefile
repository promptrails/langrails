.PHONY: all build test lint fmt vet sec coverage clean

all: fmt vet lint sec test build

## Build all packages
build:
	go build ./...

## Run all tests
test:
	go test -race -count=1 ./...

## Run tests with coverage
coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@echo ""
	@echo "HTML report: go tool cover -html=coverage.out -o coverage.html"

## Run linters
lint:
	golangci-lint run ./...

## Format code
fmt:
	gofmt -w .
	goimports -w .

## Run go vet
vet:
	go vet ./...

## Run security scanner
sec:
	gosec -quiet ./...

## Clean build artifacts
clean:
	rm -f coverage.out coverage.html
