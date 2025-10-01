.PHONY: build run test clean docker-build docker-run fmt vet lint

BINARY_NAME=rspecq-exporter
DOCKER_IMAGE=rspecq-exporter

build:
	go build -o $(BINARY_NAME) -v

run: build
	./$(BINARY_NAME)

test:
	go test -v -race -coverprofile=coverage.out ./...

coverage: test
	go tool cover -html=coverage.out

clean:
	go clean
	rm -f $(BINARY_NAME)
	rm -f coverage.out

docker-build:
	docker build -t $(DOCKER_IMAGE) .

docker-run: docker-build
	docker run -p 9292:9292 $(DOCKER_IMAGE) --redis-addr=host.docker.internal:6379

fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	golangci-lint run

deps:
	go mod download
	go mod tidy

help:
	@echo "Available targets:"
	@echo "  build        - Build the binary"
	@echo "  run          - Build and run the exporter"
	@echo "  test         - Run tests"
	@echo "  coverage     - Generate coverage report"
	@echo "  clean        - Remove built files"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-run   - Build and run Docker container"
	@echo "  fmt          - Format code"
	@echo "  vet          - Run go vet"
	@echo "  lint         - Run linter"
	@echo "  deps         - Download and tidy dependencies"
