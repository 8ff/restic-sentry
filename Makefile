VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build build-windows clean test

build:
	go build $(LDFLAGS) -o restic-sentry ./

build-windows:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o restic-sentry.exe ./

clean:
	rm -f restic-sentry restic-sentry.exe

test:
	go test ./...
