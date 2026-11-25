#!/bin/sh
set -eu
# Each sandbox receives a private copy of a trusted, prebuilt standard-lib cache.
# Participant cache files never survive or cross into another execution.
cp -R /opt/go-cache /tmp/go-cache
chmod -R u+w /tmp/go-cache
exec /usr/local/bin/run-agent
