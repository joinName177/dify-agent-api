#!/bin/sh
set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repository_root"

for required_command in docker make; do
	if ! command -v "$required_command" >/dev/null 2>&1; then
		printf 'Required command is unavailable: %s\n' "$required_command" >&2
		exit 1
	fi
done

if ! docker info >/dev/null 2>&1; then
	printf '%s\n' 'Docker daemon is unavailable. Start Docker Desktop and retry.' >&2
	exit 1
fi

if [ ! -f .env.local ]; then
	printf '%s\n' 'Missing .env.local. Copy .env.example and provide local credentials first.' >&2
	exit 1
fi

api_host_port=${API_HOST_PORT:-18080}
image_tag=${IMAGE_TAG:-dify-agent-api:dev}

case "$api_host_port" in
	'' | *[!0-9]*)
		printf 'API_HOST_PORT must be a numeric TCP port: %s\n' "$api_host_port" >&2
		exit 1
		;;
esac

if [ "$api_host_port" -lt 1 ] || [ "$api_host_port" -gt 65535 ]; then
	printf 'API_HOST_PORT must be between 1 and 65535: %s\n' "$api_host_port" >&2
	exit 1
fi

set -a
# shellcheck disable=SC1091
. ./.env.local
set +a

: "${POSTGRES_DB:?POSTGRES_DB is required in .env.local}"
: "${POSTGRES_USER:?POSTGRES_USER is required in .env.local}"
: "${POSTGRES_PASSWORD:?POSTGRES_PASSWORD is required in .env.local}"

printf '%s\n' 'Starting PostgreSQL...'
make db-up

printf '%s\n' 'Applying database migrations...'
make migrate-up

printf 'Building API image %s...\n' "$image_tag"
make IMAGE_TAG="$image_tag" build

printf 'API starting at http://localhost:%s/api/v1\n' "$api_host_port"
printf '%s\n' 'Press Ctrl+C to stop the API. PostgreSQL remains available until make db-down.'

exec docker run --rm \
	--env-file .env.local \
	--network dify-agent-api_default \
	-p "127.0.0.1:${api_host_port}:8080" \
	"$image_tag"
