#!/bin/bash
set -e

API="http://localhost"

ALICE_PASSWORD="${SEED_ALICE_PASSWORD:-AliceSecret123!}"
BOB_PASSWORD="${SEED_BOB_PASSWORD:-BobSecret123!}"

# post_json <url> [token] — reads JSON body from stdin, posts to url
post_json() {
    local url="$1"
    local token="$2"
    local data
    data=$(cat)
    if [ -n "$token" ]; then
        curl -s -X POST "$url" \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer $token" \
            --data-raw "$data"
    else
        curl -s -X POST "$url" \
            -H "Content-Type: application/json" \
            --data-raw "$data"
    fi
}

echo "Creating users..."

ALICE_RESP=$(printf '{"email":"alice@teamboard.local","password":"%s"}' "$ALICE_PASSWORD" | post_json "$API/api/v1/auth/register")
echo "  alice: $(echo "$ALICE_RESP" | grep -o '"id":"[^"]*"' | head -1)"

BOB_RESP=$(printf '{"email":"bob@teamboard.local","password":"%s"}' "$BOB_PASSWORD" | post_json "$API/api/v1/auth/register")
echo "  bob: $(echo "$BOB_RESP" | grep -o '"id":"[^"]*"' | head -1)"

echo ""
echo "Logging in as Alice..."
LOGIN_RESP=$(printf '{"email":"alice@teamboard.local","password":"%s"}' "$ALICE_PASSWORD" | post_json "$API/api/v1/auth/login")
ALICE_TOKEN=$(echo "$LOGIN_RESP" | grep -o '"access_token":"[^"]*"' | sed 's/"access_token":"//;s/"//')

if [ -z "$ALICE_TOKEN" ]; then
    echo "Login response: $LOGIN_RESP"
    echo "ERROR: Could not get Alice's token. Is the auth service running?"
    exit 1
fi

echo "Creating project 'Demo Project'..."
PROJ_RESP=$(printf '{"name":"Demo Project","description":"Created by seed script"}' | post_json "$API/api/v1/projects" "$ALICE_TOKEN")
PROJ=$(echo "$PROJ_RESP" | grep -o '"id":"[^"]*"' | head -1 | sed 's/"id":"//;s/"//')
echo "  project: $PROJ"

echo ""
echo "Seed complete."
echo "  Login: alice@teamboard.local / $ALICE_PASSWORD"
echo "  Login: bob@teamboard.local / $BOB_PASSWORD"
