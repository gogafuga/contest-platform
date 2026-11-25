#!/bin/sh
set -eu
cp -R /opt/rope-cache /work/.ropeproject
cp -R /opt/jedi-cache /tmp/cache
: > /work/main.py
exec python3 /opt/fast_server.py
