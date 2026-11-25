#!/usr/bin/env bash
# Fixed trusted fixture only. Every test stays inside gVisor, without networking.
set -euo pipefail
project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_dir"
sudo -v
sudo /usr/bin/docker image inspect contest-run-csharp:local --format 'Image: {{.Id}} | Env: {{json .Config.Env}}'
name="contest-csharp-diagnostic-$$"
cleanup() { sudo /usr/bin/docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM
variants=("${@}")
if [ "${#variants[@]}" -eq 0 ]; then variants=(fsize128 fsize512); fi
for variant in "${variants[@]}"; do
 files=256
 processes=128
 filesize=33554432
 extra=()
 case "$variant" in
  fsize128) filesize=134217728 ;;
  fsize512) filesize=536870912 ;;
  files) files=1024 ;;
  processes) processes=512 ;;
  temp) extra=(-e TMPDIR=/exec) ;;
  combined) files=1024; processes=512; extra=(-e TMPDIR=/exec) ;;
 esac
 printf '\n=== %s ===\n' "$variant"
 result_file="$(mktemp)"
 printf '%s' '{"language":"csharp","source":"using System; class Program { static void Main() { Console.Write(\"fixture-ok\"); }}","input":"","task":"diagnostic"}' |
 timeout --kill-after=5s 75s sudo /usr/bin/docker run --pull=never -i --name "$name" \
  --runtime=runsc --network=none --read-only --user=65532:65532 \
  --cap-drop=ALL --security-opt=no-new-privileges=true --log-driver=none \
  --pids-limit="$processes" --cpus=1 --memory=1024m --memory-swap=1024m \
  --ulimit="nofile=$files:$files" --ulimit="fsize=$filesize:$filesize" \
  --tmpfs=/tmp:rw,noexec,nosuid,nodev,size=384m,mode=1777 \
  --tmpfs=/work:rw,noexec,nosuid,nodev,size=32m,uid=65532,gid=65532,mode=0700 \
  --tmpfs=/exec:rw,exec,nosuid,nodev,size=64m,uid=65532,gid=65532,mode=0700 \
  "${extra[@]}" contest-run-csharp:local > "$result_file" || true
 python3 - "$result_file" <<'PY'
import base64,json,sys
try:
 data=json.load(open(sys.argv[1]))
 for key in ('stdout','stderr'):
  data[key]=base64.b64decode(data.get(key) or '').decode(errors='replace')
 print(json.dumps(data,ensure_ascii=False,indent=2))
except Exception as err: print('No agent result:',err)
PY
 rm -f "$result_file"
 sudo /usr/bin/docker inspect "$name" --format 'OOMKilled={{.State.OOMKilled}} ExitCode={{.State.ExitCode}} Error={{.State.Error}}'
 cleanup
done
