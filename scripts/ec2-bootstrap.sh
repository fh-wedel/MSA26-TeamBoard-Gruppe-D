#!/usr/bin/env bash
# One-time setup for the deployment box (Amazon Linux 2023). Idempotent.
# Can be pasted as EC2 *user-data* (runs at first boot) or run by hand:
#
#   sudo REPO_URL=https://github.com/<you>/<repo>.git bash ec2-bootstrap.sh
#
# It installs docker/compose/git, clones the repo, and generates /opt/teamboard/.env
# with random secrets. ECR registry + public IP are auto-derived at deploy time,
# so there is nothing left to edit by hand.
set -euo pipefail

REPO_URL="${REPO_URL:?Set REPO_URL=https://github.com/<you>/<repo>.git}"
TARGET="/opt/teamboard"

echo "==> Installing docker, compose plugin, git"
dnf update -y
dnf install -y docker git
systemctl enable --now docker

# Let ec2-user run docker (CI deploys over SSH as ec2-user, no sudo needed).
usermod -aG docker ec2-user

# docker compose v2 as a CLI plugin
mkdir -p /usr/local/lib/docker/cli-plugins
ARCH="$(uname -m)"  # x86_64 or aarch64
curl -fsSL "https://github.com/docker/compose/releases/latest/download/docker-compose-linux-${ARCH}" \
  -o /usr/local/lib/docker/cli-plugins/docker-compose
chmod +x /usr/local/lib/docker/cli-plugins/docker-compose

echo "==> Cloning repo into $TARGET"
if [ ! -d "$TARGET/.git" ]; then
  git clone "$REPO_URL" "$TARGET"
fi
chown -R ec2-user:ec2-user "$TARGET"

echo "==> Generating $TARGET/.env with random secrets (only if absent)"
if [ ! -f "$TARGET/.env" ]; then
  gen() { openssl rand -hex 24; }   # hex = URL-safe (these go inside DB/AMQP URLs)
  cat > "$TARGET/.env" <<EOF
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
# ECR_REGISTRY and PUBLIC_HOST are auto-derived by deploy.sh from the instance.
# Uncomment only to override:
# ECR_REGISTRY=<account>.dkr.ecr.<region>.amazonaws.com
# PUBLIC_HOST=<your-elastic-ip>
EOF
fi
chown ec2-user:ec2-user "$TARGET/.env"
chmod 600 "$TARGET/.env"

echo ""
echo "Done — the box is ready and self-configured."
echo "Set the 2 GitHub secrets (EC2_HOST = this box's public IP, EC2_SSH_KEY = your"
echo "private key) and push; CI builds, pushes to GHCR, and deploys here over SSH."
