#!/usr/bin/env sh
set -eu

echo "==> Pulling latest changes"
git pull

echo "==> Updating containers"
docker compose up -d --build

echo "==> Done"
