#!/bin/sh

set -eu

test_root=$(mktemp -d "${TMPDIR:-/tmp}/dify-agent-e2e.XXXXXX")
cleanup() {
	rm -rf -- "$test_root"
}
trap cleanup EXIT HUP INT TERM

go build -trimpath -o "$test_root/api" ./cmd/api
E2E_API_BINARY="$test_root/api" go test -tags e2e ./test/e2e
