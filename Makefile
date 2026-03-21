.PHONY: run build test clean docker-build docker-up docker-down migrate

# Run the application
run:
	go run cmd/main.go

# Build the application
build:
	go build -o bin/app cmd/main.go

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin/

# Build Docker image
docker-build:
	docker build -t girls-rating-api:latest .

# Start services with Docker Compose
docker-up:
	docker-compose up -d

# Stop services
docker-down:
	docker-compose down

# Run database migrations
migrate:
	go run cmd/migrate/main.go

# Install dependencies
deps:
	go mod download

# Generate mocks (if needed)
mocks:
	mockgen -source=internal/repository/user.go -destination=internal/repository/mocks/user_mock.go
