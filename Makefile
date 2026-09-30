.DEFAULT_GOAL := build

DOCKER_IMAGE := "ghcr.io/flared/tsnet-proxy"

.PHONY: ci
ci: \
	build \
	vet \
	test \
	format-check

.PHONY: build
build:
	go build -o tsnet-proxy .

.PHONY: test
test:
	go test -race ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: format
format:
	gofmt -w .

.PHONY: format-check
format-check:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

.PHONY: update-deps
update-deps:
	go get -u -t ./...
	go mod tidy

.PHONY: docker-build
docker-build:
	docker build -t $(DOCKER_IMAGE) .
