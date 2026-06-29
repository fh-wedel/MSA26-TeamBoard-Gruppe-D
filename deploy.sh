#!/usr/bin/env bash
# Deploy a tag on this box. Invoked by GitHub Actions over SSH with these env vars:
#     TAG, REGISTRY, GHCR_USER, GHCR_TOKEN
# The CI step provisions the box (docker, compose, git clone) before calling this;
# here we additionally generate /opt/teamboard/.env with random secrets on first run,
# so a fresh/reset EC2 box needs no manual setup. Runs as ec2-user. Idempotent.
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
REQ_TAG="${TAG:?TAG (git sha or 'latest') env var is required}"
REGISTRY="${REGISTRY:?REGISTRY env var is required, e.g. ghcr.io/<owner>}"

# Serialize deploys: a fast push could otherwise start a second deploy while an
# earlier one is still running. Wait up to 10 min for the lock.
exec 9>/tmp/teamboard-deploy.lock
flock -w 600 9 || { echo "another deploy is already running; aborting"; exit 1; }

# Generate box secrets on first run (gitignored, so they survive checkout resets).
if [ ! -f "$DIR/.env" ]; then
  echo "==> Generating $DIR/.env with random secrets"
  gen() { openssl rand -hex 24; }   # hex = URL-safe (these go inside DB/AMQP URLs)
  cat > "$DIR/.env" <<EOF
POSTGRES_USER=teamboard
POSTGRES_PASSWORD=$(gen)
RABBITMQ_USER=teamboard
RABBITMQ_PASSWORD=$(gen)
MINIO_ROOT_USER=teamboard
MINIO_ROOT_PASSWORD=$(gen)
KEY_ENCRYPTION_KEY=$(openssl rand -base64 32)
SERVICE_TOKEN_SECRET=$(openssl rand -hex 32)
SEED_ALICE_PASSWORD=AliceSecret123!
SEED_BOB_PASSWORD=BobSecret123!
EOF
  chmod 600 "$DIR/.env"
fi

# Pull the exact source revision (compose files, migrations, init scripts, traefik
# config) matching the images. The .env file is gitignored, so it survives.
git -C "$DIR" fetch --quiet --all
git -C "$DIR" reset --hard --quiet "$REQ_TAG" 2>/dev/null || \
  git -C "$DIR" reset --hard --quiet "origin/$REQ_TAG"

# Load box config (random app secrets, optional overrides).
set -a
# shellcheck disable=SC1091
source "$DIR/.env"
set +a
export TAG="$REQ_TAG"
export REGISTRY

# Public host for presigned MinIO URLs + reset links. Override in .env if not on EC2.
if [ -z "${PUBLIC_HOST:-}" ]; then
  TOK=$(curl -s -X PUT "http://169.254.169.254/latest/api/token" \
        -H "X-aws-ec2-metadata-token-ttl-seconds: 60" || true)
  PUBLIC_HOST=$(curl -s -H "X-aws-ec2-metadata-token: $TOK" \
        "http://169.254.169.254/latest/meta-data/public-ipv4" || true)
fi
export PUBLIC_HOST
echo "Registry=$REGISTRY  PublicHost=${PUBLIC_HOST:-<unset>}"

# Log in to GHCR with the short-lived token passed from CI (only if provided).
if [ -n "${GHCR_TOKEN:-}" ]; then
  echo "$GHCR_TOKEN" | docker login ghcr.io -u "${GHCR_USER:-x}" --password-stdin
fi

COMPOSE="docker compose -f $DIR/docker-compose.yml -f $DIR/docker-compose.prod.yml"
$COMPOSE pull
$COMPOSE up -d --remove-orphans

# Reclaim disk: drop images no longer used by a container (old SHA-tagged images).
docker image prune -a -f
echo "Deployed tag: $REQ_TAG"
