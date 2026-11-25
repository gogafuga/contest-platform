#!/usr/bin/env bash
set -euo pipefail
project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_dir"
case "${1:-}" in
 build)
  for language in go python csharp; do
   sudo /usr/bin/docker build -f "runners/$language/Dockerfile" -t "contest-run-$language:local" .
  done
  ;;
 start)
  mkdir -p work/bin
  (cd backend && go build -buildvcs=false -o ../work/bin/runnerd ./cmd/runnerd)
  backend_gid="$(id -g)"
  exec sudo "$project_dir/work/bin/runnerd" --gid "$backend_gid"
  ;;
 *) echo 'Usage: ./scripts/runner.sh build | start' >&2; exit 2 ;;
esac
