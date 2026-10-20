#!/usr/bin/env sh
set -eu

echo "==> Pulling latest changes"
git pull

echo "==> Testing, building and updating containers"
# The image build runs the backend and frontend tests; if any fail, the build stops and the
# running containers are left as they are.
docker compose up -d --build

echo "==> Done"
