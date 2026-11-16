.PHONY: fmt fmt-check vet arch-check staticcheck test test-race coverage coverage-check build check

fmt:
	gofmt -w ./cmd ./internal

fmt-check:
	test -z "$(shell gofmt -l ./cmd ./internal)"

vet:
	go vet ./...

arch-check:
	go test -run TestLayerImports ./internal/archtest

staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...

test:
	go test ./...

test-race:
	go test -race ./...

coverage:
	go test -coverpkg=./... -coverprofile=coverage.out ./...

coverage-check: coverage
	go tool cover -func=coverage.out | awk '/^total:/ {gsub(/%/, "", $$3); if ($$3+0 < 70) {print "coverage below 70%"; exit 1}}'

build:
	go build ./...

check:
	$(MAKE) fmt-check
	$(MAKE) vet
	$(MAKE) arch-check
	$(MAKE) test
	$(MAKE) build
