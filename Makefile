.DEFAULT_GOAL := build

DOCKER_IMAGE := "409905535292.dkr.ecr.us-east-1.amazonaws.com/tsnet-proxy"

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

.PHONY: docker-build
docker-build:
	docker build -t $(DOCKER_IMAGE) .
