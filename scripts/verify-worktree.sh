#!/bin/sh

set -u

if [ "$#" -eq 0 ]; then
	printf '%s\n' 'usage: verify-worktree.sh command [argument ...]' >&2
	exit 2
fi

repository_root=${VERIFY_REPOSITORY_ROOT:-$(git rev-parse --show-toplevel)}
repository_root=$(CDPATH= cd -- "$repository_root" && pwd)
snapshot_root=$(mktemp -d "${TMPDIR:-/tmp}/dify-agent-verify-snapshot.XXXXXX")

cleanup() {
	rm -rf -- "$snapshot_root"
}
trap cleanup EXIT HUP INT TERM

snapshot() {
	destination=$1
	mkdir -p "$destination"
	git -C "$repository_root" diff --binary --no-ext-diff HEAD > "$destination/tracked.diff"
	git -C "$repository_root" ls-files --others --exclude-standard -z > "$destination/untracked.paths"
	xargs -0 git -C "$repository_root" hash-object < "$destination/untracked.paths" > "$destination/untracked.hashes"
}

snapshot "$snapshot_root/before"

command_status=0
"$@" || command_status=$?

snapshot "$snapshot_root/after"

if ! diff -ru "$snapshot_root/before" "$snapshot_root/after"; then
	printf '%s\n' 'verify modified version-controlled or untracked workspace content:' >&2
	git -C "$repository_root" status --short >&2
	exit 1
fi

exit "$command_status"
