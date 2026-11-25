#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../.."
container=""
cleanup() {
  if [[ -n "$container" ]]; then
    docker rm -f "$container" >/dev/null
  fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# No persistent volume; Docker assigns an unused localhost port.
container=$(docker create --rm \
  --tmpfs /var/lib/postgresql/data \
  -e POSTGRES_USER=contest_test \
  -e POSTGRES_PASSWORD=contest_test \
  -e POSTGRES_DB=postgres \
  -p 127.0.0.1::5432 \
  postgres:16-alpine)
docker start "$container" >/dev/null
ready=false
for ((attempt=0; attempt<60; attempt++)); do
  if docker exec "$container" pg_isready -h 127.0.0.1 -U contest_test -d postgres >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 1
done
if [[ "$ready" != true ]]; then
  docker logs "$container"
  echo 'PostgreSQL did not become ready' >&2
  exit 1
fi
port=$(docker port "$container" 5432/tcp)
export TEST_DATABASE_URL="postgres://contest_test:contest_test@${port}/postgres?sslmode=disable"
go test -tags=integration -count=1 -timeout=3m ./tests/integration/... "$@"
