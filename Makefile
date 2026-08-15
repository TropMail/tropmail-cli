.PHONY: test lint fmt build install cover clean

BINARY := tropmail
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X github.com/tropmail/tropmail-cli/internal/cmd.Version=$(VERSION) \
	-X github.com/tropmail/tropmail-cli/internal/cmd.Commit=$(COMMIT) \
	-X github.com/tropmail/tropmail-cli/internal/cmd.Date=$(DATE)

test:
	go test ./...

lint:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
	  echo "gofmt needed:" >&2; echo "$$unformatted" >&2; exit 1; \
	fi
	go vet ./...

fmt:
	gofmt -w .

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) .

install:
	go install -trimpath -ldflags "$(LDFLAGS)" .

cover:
	go test -cover ./...

clean:
	rm -f $(BINARY)
	go clean -testcache
