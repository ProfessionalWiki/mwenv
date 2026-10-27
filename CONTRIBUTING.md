# Contributing to mwenv

You need Go 1.21 or later, which downloads the Go version mwenv needs. Run the tests with `go test ./...`.

Pushing a tag such as `v1.0.0` makes CI attach the binaries to a new release. Never move or reuse a tag: Go's module
proxy keeps serving the commit it first saw for each tag.
