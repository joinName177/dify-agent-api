#!/bin/sh

set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
fixture_root=$(mktemp -d "${TMPDIR:-/tmp}/dify-agent-hook-contract-test.XXXXXX")

cleanup() {
	rm -rf -- "$fixture_root"
}
trap cleanup EXIT HUP INT TERM

assert_fails() {
	if "$@" >/dev/null 2>&1; then
		printf 'expected command to fail: %s\n' "$*" >&2
		exit 1
	fi
}

git -C "$fixture_root" init -q
git -C "$fixture_root" config user.email test@example.invalid
git -C "$fixture_root" config user.name 'Hook Contract Test'
printf '%s\n' 'module example.test/hook' '' 'go 1.26.0' > "$fixture_root/go.mod"
printf '%s\n' 'package sample' '' 'func Value() int { return 1 }' > "$fixture_root/sample.go"
git -C "$fixture_root" add go.mod sample.go
git -C "$fixture_root" commit -qm fixture

printf '%s\n' 'package sample' '' 'func Value( )int{return 2}' > "$fixture_root/sample.go"
git -C "$fixture_root" add sample.go
assert_fails env \
	STAGED_REPOSITORY_ROOT="$fixture_root" \
	GO_IMAGE="$GO_IMAGE" \
	GITLEAKS_IMAGE="$GITLEAKS_IMAGE" \
	"$repository_root/scripts/staged-check.sh"

printf '%s\n' 'package sample' '' 'func Value() int { return 2 }' > "$fixture_root/sample.go"
git -C "$fixture_root" add sample.go
printf '%s\n' 'package sample' '' 'func Value( )int{return 3}' > "$fixture_root/sample.go"
STAGED_REPOSITORY_ROOT="$fixture_root" \
	GO_IMAGE="$GO_IMAGE" \
	GITLEAKS_IMAGE="$GITLEAKS_IMAGE" \
	"$repository_root/scripts/staged-check.sh" >/dev/null

printf '%s\n' 'GITLAB_TOKEN=glpat-1234567890abcdefghij' > "$fixture_root/secret.txt"
git -C "$fixture_root" add secret.txt
assert_fails env \
	STAGED_REPOSITORY_ROOT="$fixture_root" \
	GO_IMAGE="$GO_IMAGE" \
	GITLEAKS_IMAGE="$GITLEAKS_IMAGE" \
	"$repository_root/scripts/staged-check.sh"

git -C "$fixture_root" reset -q HEAD secret.txt
git -C "$fixture_root" update-index --force-remove sample.go
STAGED_REPOSITORY_ROOT="$fixture_root" \
	GO_IMAGE="$GO_IMAGE" \
	GITLEAKS_IMAGE="$GITLEAKS_IMAGE" \
	"$repository_root/scripts/staged-check.sh" >/dev/null

printf '%s\n' 'fix(auth): reject expired sessions' > "$fixture_root/COMMIT_EDITMSG"
(cd "$repository_root" && .githooks/commit-msg "$fixture_root/COMMIT_EDITMSG") >/dev/null
printf '%s\n' 'invalid title' > "$fixture_root/COMMIT_EDITMSG"
assert_fails sh -c "cd '$repository_root' && .githooks/commit-msg '$fixture_root/COMMIT_EDITMSG'"

printf '%s\n' 'hook contract tests passed'
