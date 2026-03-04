VERSION ?= dev
BINARY  := gids
DIST    := dist

LDFLAGS_BASE := -X gids/internal/version.Version=$(VERSION)
LDFLAGS_DEV  := -ldflags "$(LDFLAGS_BASE)"

.PHONY: build test test-coverage coverage-html coverage-terminal vet run snapshot clean

build:
	@mkdir -p bin
	go build $(LDFLAGS_DEV) -o bin/$(BINARY) .

# test release builds locally 
snapshot:
	goreleaser build --snapshot --clean --single-target

run:
	go run $(LDFLAGS_DEV) . $(ARGS)

test:
	go test ./...

vet:
	go vet ./...

test-coverage:
	@mkdir -p coverage
	go test -v -coverprofile=coverage/coverage.out ./...

coverage-html:
	@if [ ! -f coverage/coverage.out ]; then \
		echo "Generating coverage..."; \
		$(MAKE) test-coverage; \
	fi
	go tool cover -html=coverage/coverage.out

coverage-terminal:
	@if [ ! -f coverage/coverage.out ]; then \
		echo "Generating coverage..."; \
		$(MAKE) test-coverage; \
	fi
	@go tool cover -func=coverage/coverage.out

clean:
	rm -rf bin/ coverage/ $(DIST)/
