#!/bin/sh
set -eu
compiler="$(cat /opt/contest-csc-path)"
exec /usr/bin/dotnet exec "$compiler" -noconfig @/opt/contest-csharp.rsp
