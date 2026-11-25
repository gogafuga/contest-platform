#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

usage() {
  echo "Usage: $0 start [LAN_IP] | stop"
  echo 'start: wipe previous contest data and launch a fresh contest'
  echo 'stop: stop services and preserve contest data'
}
case "${1:-}" in
  start) shift ;;
  stop)
    if [[ $# -ne 1 ]]; then usage >&2; exit 1; fi
    exec docker compose stop
    ;;
  -h|--help) usage; exit 0 ;;
  *) usage >&2; exit 1 ;;
esac

contest_host="${1:-}"
if [[ -z "$contest_host" ]] && command -v ipconfig >/dev/null; then
  contest_host="$(ipconfig getifaddr en0 2>/dev/null || true)"
fi
contest_host="${contest_host:-localhost}"
export PORT="${PORT:-80}"
if [[ $# -gt 1 || ! "$contest_host" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*$ || ! "$PORT" =~ ^[0-9]+$ ]]; then
  usage >&2
  exit 1
fi
if (( PORT < 1 || PORT > 65535 )); then echo 'PORT must be between 1 and 65535' >&2; exit 1; fi
command -v docker >/dev/null
docker compose version >/dev/null
# Build/cache images beforehand; launching offline must never pull or rebuild.
docker image inspect contest01:local postgres:16-alpine >/dev/null
if [[ -z "${ADMIN_TOKEN:-}" ]]; then
  export ADMIN_TOKEN="$(docker run --rm --pull never --entrypoint sh contest01:local -c 'head -c 32 /dev/urandom | od -An -tx1 | tr -d " \n"')"
fi
if [[ -z "$ADMIN_TOKEN" ]]; then echo 'Could not generate admin token' >&2; exit 1; fi

echo 'Resetting contest: previous participants, sessions, scores and clock will be erased.'
docker compose down --volumes
# Keep containers/data on failure for diagnosis; never remove the volume on exit.
if ! docker compose up -d --no-build --pull never --wait --wait-timeout 90; then
  docker compose logs --tail 80
  exit 1
fi
contest_url="http://$contest_host"
if [[ "$PORT" != 80 ]]; then contest_url="$contest_url:$PORT"; fi
printf '\nContest: %s\nClock admin: %s/clock\nAdmin token: %s\n' \
  "$contest_url" "$contest_url" "$ADMIN_TOKEN"
echo 'UI and API use the same address. No LAN address is baked into the image.'
echo 'Services run in the background. Stop: ./scripts/contest.sh stop'
echo 'Running ./scripts/contest.sh start again erases contest data.'
if [[ "$contest_host" == localhost ]]; then echo 'Using localhost. For phone/student access, pass your laptop LAN IP explicitly.'; fi
