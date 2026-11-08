VERSION ?= dev
BINARY  := gids

LDFLAGS_BASE := -X gids/internal/version.version=$(VERSION)
LDFLAGS_DEV  := -ldflags "$(LDFLAGS_BASE)"

.PHONY: build test test-short test-coverage coverage-html coverage-terminal vet fmt vulncheck verify sandbox run snapshot

build:
	@mkdir -p bin
	go build $(LDFLAGS_DEV) -o bin/$(BINARY) .

# test release builds locally 
snapshot:
	goreleaser build --snapshot --clean --single-target

run:
	go run $(LDFLAGS_DEV) . $(ARGS)

test:
	go test -race ./...

# Skips the slow tests (crash at every step).
test-short:
	go test -short ./...

fmt:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }

# Everything in verify/README.md: the full suite plus the exhaustive state
# search and the TLA+ models (needs java and verify/tla/tla2tools.jar).
verify:
	GIDS_VERIFY=1 go test -race -timeout 30m ./...
	./verify/tla/run.sh

# No known vulnerabilities in reachable code, any required module, or the toolchain.
vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -scan module

# A shell whose HOME, gids dir and global git config are a throwaway temp
# dir, with the freshly built gids first on PATH. Try anything: your real
# ~/.gitconfig is never touched (SPEC §11.1).
sandbox: build
	@set -eu; d=$$(mktemp -d "$${TMPDIR:-/tmp}/gids-sandbox.XXXXXX"); \
	case "$$d" in ""|/|"$$HOME"|"$$HOME"/*) echo "refusing: unsafe sandbox dir '$$d'" >&2; exit 1;; esac; \
	test -d "$$d" || { echo "refusing: sandbox dir missing" >&2; exit 1; }; \
	mkdir -p "$$d/bin"; cp bin/$(BINARY) "$$d/bin/"; \
	printf '[user]\n\tname = Sandbox User\n\temail = sandbox@example.com\n' > "$$d/.gitconfig"; \
	echo "gids sandbox: HOME=$$d  (exit the shell to leave)"; \
	cd "$$d" && HOME="$$d" XDG_CONFIG_HOME="$$d/.config" GIT_CONFIG_GLOBAL="$$d/.gitconfig" \
	  GIT_CONFIG_NOSYSTEM=1 PATH="$$d/bin:$$PATH" sh -i

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
