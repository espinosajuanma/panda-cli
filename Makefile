.PHONY: fmt fmt-check vet test test-race build check

fmt:
	gofmt -w ./cmd ./internal

fmt-check:
	test -z "$(shell gofmt -l ./cmd ./internal)"

vet:
	go vet ./...

test:
	go test ./...

test-race:
	go test -race ./...

build:
	go build ./...

check:
	$(MAKE) fmt-check
	$(MAKE) vet
	$(MAKE) test
	$(MAKE) build
