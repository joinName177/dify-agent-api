#!/bin/sh

set -eu

tool_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
repository_root=${STAGED_REPOSITORY_ROOT:-$(git rev-parse --show-toplevel)}
repository_root=$(CDPATH= cd -- "$repository_root" && pwd)
temporary_root=$(mktemp -d "${TMPDIR:-/tmp}/dify-agent-staged-check.XXXXXX")

cleanup() {
	rm -rf -- "$temporary_root"
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$temporary_root/worktree"
git -C "$repository_root" checkout-index --all --prefix="$temporary_root/worktree/"
git -C "$repository_root" diff --cached --name-only --diff-filter=ACMR -z > "$temporary_root/staged.paths"

docker run --rm \
	-v "$tool_root:/src:ro" \
	-v "$temporary_root:/snapshot:ro" \
	-w /src \
	"$GO_IMAGE" \
	go run ./tools/stagedcheck -root /snapshot/worktree -paths /snapshot/staged.paths

docker run --rm \
	-v "$repository_root:/repo:ro" \
	-w /repo \
	"$GITLEAKS_IMAGE" \
	git --pre-commit --staged --redact --no-banner .

if git -C "$repository_root" diff --cached --quiet -- '*.go' go.mod go.sum; then
	printf '%s\n' 'No staged Go or module changes; affected tests skipped.'
	exit 0
fi

docker run --rm \
	-v "$temporary_root/worktree:/src:ro" \
	-w /src \
	"$GO_IMAGE" \
	sh -c 'packages=$(go list ./...); if [ -z "$packages" ]; then printf "%s\n" "No Go packages remain in the staged snapshot; tests skipped."; else go test $packages; fi'
