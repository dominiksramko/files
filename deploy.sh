#!/usr/bin/env sh
set -eu

# The image docker-compose.yml builds; each deploy also tags it with its commit.
IMAGE=files

echo "==> Pulling latest changes"
git pull

# What Settings shows: the latest release tag (v1.2.0), with the commits since it when there
# are any (v1.2.0-3-g849a251), and -dirty when built with uncommitted changes. Without any
# tag yet, just the commit. `git pull` brings the tags along.
export APP_VERSION
APP_VERSION=$(git describe --tags --always --dirty)

echo "==> Testing, building and updating containers ($APP_VERSION)"
# The image build runs the backend and frontend tests; if any fail, the build stops and the
# running containers are left as they are.
docker compose up -d --build

commit=$(git rev-parse --short HEAD)
echo "==> Tagging the image as $IMAGE:$commit"
docker tag "$IMAGE:latest" "$IMAGE:$commit"

echo "==> Removing images no longer needed"
# Keep this commit's image and the newest one before it (to roll back to); `docker image ls`
# lists newest first. Then drop this app's untagged images, e.g. from a commit built twice.
docker image ls "$IMAGE" --format '{{.Tag}}' \
  | grep -vx -e latest -e "$commit" -e '<none>' \
  | tail -n +2 \
  | while read -r old; do
      docker image rm "$IMAGE:$old" >/dev/null && echo "    removed $IMAGE:$old" \
        || echo "    kept $IMAGE:$old (still used by a container)"
    done
docker image prune -f --filter "label=org.opencontainers.image.title=files" >/dev/null

echo "==> Done"
