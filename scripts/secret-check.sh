#!/bin/sh

set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
snapshot_root=$(mktemp -d "${TMPDIR:-/tmp}/dify-agent-secret-snapshot.XXXXXX")

cleanup() {
	rm -rf -- "$snapshot_root"
}
trap cleanup EXIT HUP INT TERM

docker run --rm \
	-v "$repository_root:/repo:ro" \
	-w /repo \
	"$GITLEAKS_IMAGE" \
	git --config /repo/.gitleaks.toml --redact --no-banner .

git -C "$repository_root" ls-files -co --exclude-standard | while IFS= read -r path; do
	if [ -f "$repository_root/$path" ]; then
		mkdir -p "$snapshot_root/$(dirname -- "$path")"
		cp "$repository_root/$path" "$snapshot_root/$path"
	fi
done

docker run --rm \
	-v "$snapshot_root:/snapshot:ro" \
	-v "$repository_root/.gitleaks.toml:/config/.gitleaks.toml:ro" \
	-w /snapshot \
	"$GITLEAKS_IMAGE" \
	dir --config /config/.gitleaks.toml --redact --no-banner .
