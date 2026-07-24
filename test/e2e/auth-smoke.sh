#!/bin/sh
set -eu

: "${BASE_URL:?set BASE_URL}"
: "${ADMIN_EMAIL:?set ADMIN_EMAIL}"
: "${ADMIN_PASSWORD:?set ADMIN_PASSWORD}"

test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
origin=http://localhost:3000
user_email="user-$(date +%s)@example.test"
user_password=integration-user-password

request() {
  output=$1
  shift
  curl --silent --show-error --output "$test_dir/$output.body" --write-out '%{http_code}' "$@"
}

status=$(request version "$BASE_URL/version")
[ "$status" = 200 ]
jq -e '.contractId == "dify-agent-api/v1" and (.openapiDigest | length == 64)' "$test_dir/version.body" >/dev/null

status=$(request register -c "$test_dir/session.cookie" -H "Origin: $origin" -H 'Content-Type: application/json' --data "{\"email\":\"$user_email\",\"password\":\"$user_password\",\"displayName\":\"Integration User\"}" "$BASE_URL/auth/register")
[ "$status" = 201 ]
access_token=$(jq -r '.accessToken' "$test_dir/register.body")
jq -e --arg email "$user_email" '.user.email == $email and .user.role == "user"' "$test_dir/register.body" >/dev/null
cp "$test_dir/session.cookie" "$test_dir/old-session.cookie"

status=$(request me -H "Authorization: Bearer $access_token" "$BASE_URL/auth/me")
[ "$status" = 200 ]
jq -e --arg email "$user_email" '.email == $email' "$test_dir/me.body" >/dev/null

status=$(request refresh -b "$test_dir/session.cookie" -c "$test_dir/session.cookie" -H "Origin: $origin" -X POST "$BASE_URL/auth/refresh")
[ "$status" = 200 ]
rotated_access_token=$(jq -r '.accessToken' "$test_dir/refresh.body")
[ "$rotated_access_token" != "$access_token" ]

status=$(request reused-refresh -b "$test_dir/old-session.cookie" -H "Origin: $origin" -X POST "$BASE_URL/auth/refresh")
[ "$status" = 401 ]

status=$(request duplicate -H "Origin: $origin" -H 'Content-Type: application/json' --data "{\"email\":\"$user_email\",\"password\":\"$user_password\",\"displayName\":\"Duplicate\"}" "$BASE_URL/auth/register")
[ "$status" = 409 ]
jq -e '.code == "EMAIL_ALREADY_REGISTERED"' "$test_dir/duplicate.body" >/dev/null

status=$(request admin -H "Origin: $origin" -H 'Content-Type: application/json' --data "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" "$BASE_URL/auth/login")
[ "$status" = 200 ]
jq -e '.user.role == "super_admin"' "$test_dir/admin.body" >/dev/null

status=$(request logout -b "$test_dir/session.cookie" -H "Origin: $origin" -X POST "$BASE_URL/auth/logout")
[ "$status" = 204 ]
status=$(request revoked-access -H "Authorization: Bearer $rotated_access_token" "$BASE_URL/auth/me")
[ "$status" = 401 ]

echo "authentication E2E passed for $user_email"
