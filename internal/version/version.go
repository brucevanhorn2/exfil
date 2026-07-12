// Package version holds the build-time version string. Version is
// overridden via -ldflags "-X ...Version=..." at build time (see Makefile);
// it stays "dev" for plain `go build`/`go run`.
package version

var Version = "dev"
