#!/usr/bin/env sh
set -eu

# The image docker-compose.yml builds; each deploy also tags it with its commit.
IMAGE=files

echo "==> Pulling latest changes"
git pull

echo "==> Testing, building and updating containers"
# The image build runs the backend and frontend tests; if any fail, the build stops and the
# running containers are left as they are.
docker compose up -d --build

version=$(git rev-parse --short HEAD)
echo "==> Tagging the image as $IMAGE:$version"
docker tag "$IMAGE:latest" "$IMAGE:$version"

echo "==> Removing images no longer needed"
# Keep this version and the newest one before it (to roll back to); `docker image ls`
# lists newest first. Then drop this app's untagged images, e.g. from a commit built twice.
docker image ls "$IMAGE" --format '{{.Tag}}' \
  | grep -vx -e latest -e "$version" -e '<none>' \
  | tail -n +2 \
  | while read -r old; do
      docker image rm "$IMAGE:$old" >/dev/null && echo "    removed $IMAGE:$old" \
        || echo "    kept $IMAGE:$old (still used by a container)"
    done
docker image prune -f --filter "label=org.opencontainers.image.title=files" >/dev/null

echo "==> Done"
