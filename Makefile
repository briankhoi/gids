VERSION ?= dev
LDFLAGS  := -ldflags "-X gids/internal/version.Version=$(VERSION)"
BINARY   := bin/gids

PLATFORMS := linux/amd64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: build test vet run clean build-all install

build:
	go build $(LDFLAGS) -o $(BINARY) .

test:
	go test ./...

test-verbose:
	go test -v ./...

vet:
	go vet ./...

run:
	go run $(LDFLAGS) . $(ARGS)

clean:
	rm -rf bin/

build-all:
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*} GOARCH=$${platform#*/} ; \
		out=$(BINARY)-$${platform%/*}-$${platform#*/} ; \
		if [ "$${platform%/*}" = "windows" ]; then out=$$out.exe ; fi ; \
		echo "Building $$out" ; \
		GOOS=$${platform%/*} GOARCH=$${platform#*/} go build $(LDFLAGS) -o $$out . ; \
	done

install:
	go install $(LDFLAGS) .
