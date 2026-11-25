#!/usr/bin/env bash
set -euo pipefail
project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_dir"
# Rebuild changed images, then keep the private analyzer supervisor in foreground.
sudo /usr/bin/docker build -f runners/go/Dockerfile -t contest-run-go:local .
sudo /usr/bin/docker build -t contest-csharp-ls:local analyzers/csharp
exec ./scripts/analyzers.sh start --runtime runsc
