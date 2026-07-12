VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/bvanhorn/exfil/internal/version.Version=$(VERSION)

.PHONY: build
build:
	go build -ldflags "$(LDFLAGS)" -o exfil ./cmd/exfil

.PHONY: clean
clean:
	rm -f exfil
