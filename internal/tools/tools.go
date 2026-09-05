//go:build tools

// Package tools pins development tool dependencies so their versions are
// recorded in go.mod/go.sum and `make tools` installs deterministic versions.
package tools

import (
	_ "golang.org/x/tools/cmd/goimports"
	_ "golang.org/x/vuln/cmd/govulncheck"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
	_ "honnef.co/go/tools/cmd/staticcheck"
)
