#!/bin/sh

set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
fixture_root=$(mktemp -d "${TMPDIR:-/tmp}/dify-agent-build-contract-test.XXXXXX")

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

mkdir -p "$fixture_root/generated/api/openapi/v1" "$fixture_root/generated/generated/openapi/v1"
cp "$repository_root/api/openapi/v1/openapi.yaml" "$fixture_root/generated/api/openapi/v1/openapi.yaml"
cp "$repository_root/api/openapi/v1/oapi-codegen.yaml" "$fixture_root/generated/api/openapi/v1/oapi-codegen.yaml"
cp "$repository_root/api/openapi/v1/openapi.sha256" "$fixture_root/generated/api/openapi/v1/openapi.sha256"
cp "$repository_root/generated/openapi/v1/server.gen.go" "$fixture_root/generated/generated/openapi/v1/server.gen.go"
cp "$repository_root/generated/openapi/v1/digest.gen.go" "$fixture_root/generated/generated/openapi/v1/digest.gen.go"

printf '%s\n' '// deliberate generated drift' >> "$fixture_root/generated/generated/openapi/v1/server.gen.go"
assert_fails env \
	GO_IMAGE="$GO_IMAGE" \
	OAPI_CODEGEN_VERSION="$OAPI_CODEGEN_VERSION" \
	"$repository_root/scripts/generated-check.sh" "$fixture_root/generated"

cp "$repository_root/generated/openapi/v1/server.gen.go" "$fixture_root/generated/generated/openapi/v1/server.gen.go"
printf '\n' >> "$fixture_root/generated/api/openapi/v1/openapi.yaml"
assert_fails env \
	GO_IMAGE="$GO_IMAGE" \
	OAPI_CODEGEN_VERSION="$OAPI_CODEGEN_VERSION" \
	"$repository_root/scripts/generated-check.sh" "$fixture_root/generated"

mkdir -p "$fixture_root/worktree"
git -C "$fixture_root/worktree" init -q
git -C "$fixture_root/worktree" config user.email test@example.invalid
git -C "$fixture_root/worktree" config user.name 'Build Contract Test'
printf '%s\n' 'before' > "$fixture_root/worktree/tracked.txt"
git -C "$fixture_root/worktree" add tracked.txt
git -C "$fixture_root/worktree" commit -qm fixture

VERIFY_REPOSITORY_ROOT="$fixture_root/worktree" \
	"$repository_root/scripts/verify-worktree.sh" true
assert_fails env \
	VERIFY_REPOSITORY_ROOT="$fixture_root/worktree" \
	"$repository_root/scripts/verify-worktree.sh" \
	sh -c "printf '%s\\n' 'after' > '$fixture_root/worktree/tracked.txt'"

git -C "$fixture_root/worktree" restore tracked.txt
printf '%s\n' 'untracked-before' > "$fixture_root/worktree/untracked.txt"
assert_fails env \
	VERIFY_REPOSITORY_ROOT="$fixture_root/worktree" \
	"$repository_root/scripts/verify-worktree.sh" \
	sh -c "printf '%s\\n' 'untracked-after' > '$fixture_root/worktree/untracked.txt'"

mkdir -p "$fixture_root/make"
cp "$repository_root/Makefile" "$fixture_root/make/Makefile"
cp "$repository_root/go.mod" "$fixture_root/make/go.mod"
cp "$repository_root/go.sum" "$fixture_root/make/go.sum"
cp -R "$repository_root/cmd" "$fixture_root/make/cmd"
cp -R "$repository_root/internal" "$fixture_root/make/internal"
cp -R "$repository_root/generated" "$fixture_root/make/generated"
cp -R "$repository_root/tools" "$fixture_root/make/tools"

printf 'package governancecheck\n\nfunc deliberatelyUnformatted( ){ }\n' > "$fixture_root/make/tools/governancecheck/format_negative.go"
assert_fails make -C "$fixture_root/make" format-check
rm "$fixture_root/make/tools/governancecheck/format_negative.go"

printf '\nrequire example.com/unused v0.0.0\n' >> "$fixture_root/make/go.mod"
assert_fails make -C "$fixture_root/make" mod-check

printf '%s\n' 'build command contract tests passed'
