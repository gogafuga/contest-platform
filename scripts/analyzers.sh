#!/usr/bin/env bash
# Local development supervisor. The public API is always run separately as a user.
set -euo pipefail
project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_dir"
case "${1:-}" in
  build)
    sudo /usr/bin/docker build -t contest-gopls:local analyzers/go
    sudo /usr/bin/docker build -t contest-pylsp:local analyzers/python
    sudo /usr/bin/docker build -t contest-csharp-ls:local analyzers/csharp
    ;;
  start)
    mkdir -p work/bin
    (cd backend && go build -buildvcs=false -o ../work/bin/analyzerd ./cmd/analyzerd)
    backend_gid="$(id -g)"
    exec sudo "$project_dir/work/bin/analyzerd" --gid "$backend_gid" "${@:2}"
    ;;
  *) echo "Usage: ./scripts/analyzers.sh build | start [--runtime runsc] [--max-sessions 2]" >&2; exit 2 ;;
esac
