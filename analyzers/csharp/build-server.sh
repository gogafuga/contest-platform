#!/bin/sh
# Build only the pinned upstream analyzer. No participant code is present here.
set -eu
output=${1:?absolute publish output directory required}
archive=${2:?pinned source archive required}
case "$output" in /*) ;; *) echo 'Publish output must be absolute' >&2; exit 2 ;; esac
case "${CSHARP_READYTORUN:-true}" in true|false) ;; *) echo 'CSHARP_READYTORUN must be true or false' >&2; exit 2 ;; esac
case "$(uname -m)" in
  x86_64) rid=linux-x64 ;;
  aarch64) rid=linux-arm64 ;;
  *) echo 'Unsupported C# analyzer architecture' >&2; exit 2 ;;
esac
printf '%s  %s\n' a9a34dccff8b4aee2c1f0b8c3f2a3fd0361dfd55398385f465cc4e8f238045b4 "$archive" | sha256sum -c -
build_dir=$(mktemp -d)
trap 'rm -rf "$build_dir"' EXIT
trap 'exit 130' HUP INT TERM
tar -xzf "$archive" -C "$build_dir" --strip-components=1

# With recent SDKs, the vendored LSP project otherwise picks the SDK's FSharp.Core
# implicitly, conflicting with upstream's centrally pinned 10.0.101. Reference
# that existing central version explicitly; no dependency versions are upgraded.
protocol="$build_dir/src/Ionide.LanguageServerProtocol/src/Ionide.LanguageServerProtocol.fsproj"
sed -i '/<PackageReference Include="System.Text.Json"/i\    <PackageReference Include="FSharp.Core" />' "$protocol"
grep -Fq '<PackageReference Include="FSharp.Core" />' "$protocol"
(
  cd "$build_dir"
  dotnet publish src/CSharpLanguageServer/CSharpLanguageServer.fsproj \
    -c Release -r "$rid" --self-contained false \
    -p:PackAsTool=false -p:DebugType=None -p:DebugSymbols=false \
    "-p:PublishReadyToRun=${CSHARP_READYTORUN:-true}" \
    -o "$output"
)
cp "$build_dir/LICENSE" "$output/LICENSE.csharp-ls"
test -f "$output/CSharpLanguageServer.dll"
chmod -R a+rX "$output"
dotnet "$output/CSharpLanguageServer.dll" --version
