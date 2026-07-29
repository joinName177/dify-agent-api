#!/bin/sh

set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
fixture_root=$(mktemp -d "${TMPDIR:-/tmp}/dify-agent-secret-contract.XXXXXX")

cleanup() {
	rm -rf -- "$fixture_root"
}
trap cleanup EXIT HUP INT TERM

assert_fixture_fails() {
	fixture_name=$1
	fixture_value=$2
	test_repository="$fixture_root/$fixture_name"
	mkdir -p "$test_repository"
	git -C "$test_repository" init -q
	git -C "$test_repository" config user.email test@example.invalid
	git -C "$test_repository" config user.name 'Secret Contract Test'
	printf '%s\n' "$fixture_value" > "$test_repository/secret.txt"
	git -C "$test_repository" add secret.txt
	git -C "$test_repository" commit -qm fixture
	if docker run --rm \
		-v "$test_repository:/repo:ro" \
		-v "$repository_root/.gitleaks.toml:/repo/.gitleaks.toml:ro" \
		-w /repo \
		"$GITLEAKS_IMAGE" \
		git --config /repo/.gitleaks.toml --redact --no-banner . >/dev/null 2>&1; then
		printf 'gitleaks accepted %s fixture\n' "$fixture_name" >&2
		exit 1
	fi
}

assert_fixture_fails private-key '-----BEGIN RSA PRIVATE KEY-----'
assert_fixture_fails github-token 'GITHUB_TOKEN=ghp_123456789012345678901234567890123456'
assert_fixture_fails database-url 'DATABASE_URL=postgres://admin:sensitive-password@db.example.test/app'
assert_fixture_fails generic-api-key 'api_key = "0123456789abcdefghijklmnopqrstuvwxyz"'

printf '%s\n' 'secret scanning contract tests passed'
