.PHONY: tidy build test test-race run

tidy:
	cd backend && go mod tidy

build:
	cd backend && go build -o bin/telemetrypulse ./cmd/telemetrypulse/

test:
	cd backend && go test ./...

test-race:
	cd backend && go test -race -v ./...

test-unit:
	cd backend && go test -v ./internal/window/... ./internal/anomaly/...

test-integration:
	cd backend && go test -v ./internal/telemetry/...

run:
	cd backend && go run ./cmd/telemetrypulse/

vet:
	cd backend && go vet ./...
