#!/bin/sh
set -eu
if [ "$#" -gt 0 ]; then exec /usr/local/bin/gopls "$@"; fi
mkdir -p /tmp/go-cache
tar -xf /opt/go-cache.tar -C /tmp/go-cache
printf 'module contest.local/solution\n\ngo 1.27.0\n' > /work/go.mod
printf 'package main\nfunc main() {}\n' > /work/main.go
exec /usr/local/bin/gopls serve
