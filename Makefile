.PHONY: run build test docker-up docker-down dev install-air

# Run the application directly 
api:
	go run cmd/api/main.go

rabbit:
	go run cmd/rabbit/main.go

pubg:
	go run cmd/pubg/main.go

unplayable:
	go run cmd/unplayable/main.go

# Creates initial admin token based off of config.json
token:
	go run cmd/tokengen/main.go

# Build the application
build:
	go build -o bin/api cmd/api/main.go

# Run tests
test:
	go test -v ./...

# Start Docker Compose services
docker-up:
	docker-compose up -d

# Stop Docker Compose services
docker-down:
	docker-compose down

# Install Air for hot reloading
install-air:
	go install github.com/cosmtrek/air@latest

# Run with hot reloading using Air
dev:
	air

tidy:
	go mod tidy