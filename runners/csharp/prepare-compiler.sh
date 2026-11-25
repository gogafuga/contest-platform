#!/bin/sh
# Build-time preparation only. Runtime source paths and options are fixed.
set -eu
sdk_version="$(dotnet --version)"
compiler="/usr/share/dotnet/sdk/$sdk_version/Roslyn/bincore/csc.dll"
test -f "$compiler"
printf '%s\n' "$compiler" > /opt/contest-csc-path
set -- /usr/share/dotnet/packs/Microsoft.NETCore.App.Ref/*/ref/net10.0
# The SDK image contains the matching framework reference pack.
[ "$#" -eq 1 ] && test -d "$1"
reference_dir="$1"
{
 printf '%s\n' '-nologo' '-nostdlib+' '-target:exe' '-out:/exec/Solution.dll' '-optimize+' '-debug-' '-deterministic+' '-langversion:14.0' '-nullable:disable'
 for reference in "$reference_dir"/*.dll; do printf '%s\n' "-reference:$reference"; done
 printf '%s\n' '/work/Program.cs'
} > /opt/contest-csharp.rsp
printf '%s\n' '{"runtimeOptions":{"tfm":"net10.0","framework":{"name":"Microsoft.NETCore.App","version":"10.0.0"}}}' > /opt/Solution.runtimeconfig.json
chmod a+r /opt/contest-csc-path /opt/contest-csharp.rsp /opt/Solution.runtimeconfig.json
