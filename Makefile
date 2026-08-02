.PHONY: build lint fmt test test-integration generate docker vulncheck ci

build:
	go build ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -l .

test:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

test-integration:
	go test -race -tags integration -timeout 120s ./...

generate:
	go generate ./internal/api/v1/...
	git diff --exit-code -- internal/api/v1/server.gen.go

docker:
	docker build -t inwheel-api:local -f cmd/api/Dockerfile .

vulncheck:
	govulncheck ./...

ci: build lint generate test test-integration
