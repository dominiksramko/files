#!/usr/bin/env sh
set -eu

# The development stack (docker-compose.dev.yml): Go backend with live reload + Vite on :5173.
#
#   ./dev.sh                  start it (or rebuild it) and follow the logs; Ctrl-C leaves it running
#   ./dev.sh down             stop it
#   ./dev.sh logs [service]   follow the logs
#   ./dev.sh test             backend and frontend tests
#   ./dev.sh lint             go vet + the frontend linters (which fix what they can)
#   ./dev.sh migrate <name>   create a database migration
#   ./dev.sh sh <service>     a shell in backend or frontend
#   ./dev.sh <anything else>  passed to docker compose, e.g. ./dev.sh exec frontend npm install x

cd "$(dirname "$0")"

# The containers run as you, so the files they create (backend/data, node_modules, lint
# fixes) stay yours.
DEV_UID=${DEV_UID:-$(id -u)}
DEV_GID=${DEV_GID:-$(id -g)}
export DEV_UID DEV_GID

compose() {
  docker compose -f docker-compose.dev.yml "$@"
}

command=${1:-up}
[ $# -gt 0 ] && shift

case "$command" in
  up)
    compose up -d --build "$@"
    echo "==> Running at http://localhost:5173 (admin / admin); ./dev.sh down stops it"
    compose logs -f
    ;;
  test)
    # Both suites run even when the first fails; the exit status says whether either did.
    status=0
    echo "==> Backend tests"
    compose exec backend go test ./... || status=1
    echo "==> Frontend tests"
    compose exec frontend npm test || status=1
    exit "$status"
    ;;
  lint)
    status=0
    echo "==> go vet"
    compose exec backend go vet ./... || status=1
    echo "==> Frontend lint"
    compose exec frontend npm run lint || status=1
    exit "$status"
    ;;
  migrate)
    [ $# -eq 1 ] || { echo "usage: ./dev.sh migrate <name>" >&2; exit 2; }
    compose exec backend go run ./cmd/migrate "$1"
    ;;
  sh)
    [ $# -eq 1 ] || { echo "usage: ./dev.sh sh backend|frontend" >&2; exit 2; }
    compose exec "$1" sh
    ;;
  logs)
    compose logs -f "$@"
    ;;
  *)
    compose "$command" "$@"
    ;;
esac
