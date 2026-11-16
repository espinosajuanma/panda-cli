.PHONY: fmt fmt-check vet arch-check test test-race build check

fmt:
	gofmt -w ./cmd ./internal

fmt-check:
	test -z "$(shell gofmt -l ./cmd ./internal)"

vet:
	go vet ./...

arch-check:
	go test -run TestLayerImports ./internal/archtest

test:
	go test ./...

test-race:
	go test -race ./...

build:
	go build ./...

check:
	$(MAKE) fmt-check
	$(MAKE) vet
	$(MAKE) arch-check
	$(MAKE) test
	$(MAKE) build
