#!/bin/sh

set -eu

: "${IMAGE_TAG:?set IMAGE_TAG}"
: "${GRYPE_IMAGE:?set GRYPE_IMAGE}"

image_user=$(docker image inspect --format '{{.Config.User}}' "$IMAGE_TAG")
image_entrypoint=$(docker image inspect --format '{{json .Config.Entrypoint}}' "$IMAGE_TAG")
if [ "$image_user" != "nonroot:nonroot" ] || [ "$image_entrypoint" != '["/api"]' ]; then
	printf 'unsafe final image metadata: user=%s entrypoint=%s\n' "$image_user" "$image_entrypoint" >&2
	exit 1
fi

container_id=$(docker create "$IMAGE_TAG")
archive_file=$(mktemp "${TMPDIR:-/tmp}/dify-agent-image.XXXXXX.tar")
cleanup() {
	docker rm "$container_id" >/dev/null 2>&1 || true
	rm -f -- "$archive_file"
}
trap cleanup EXIT HUP INT TERM
docker export --output "$archive_file" "$container_id"

if tar -tf "$archive_file" | grep -E '(^|/)(\.git|\.env([^/]*)?|go\.mod|go\.sum|[^/]+\.go|migrations|test)(/|$)' >/dev/null; then
	printf '%s\n' 'final image contains source, repository metadata, migration, test, or environment files' >&2
	exit 1
fi

docker run --rm \
	-v /var/run/docker.sock:/var/run/docker.sock \
	"$GRYPE_IMAGE" \
	"$IMAGE_TAG" --fail-on high --only-fixed
