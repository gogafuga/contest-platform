#!/usr/bin/env bash
# Local Ubuntu/systemd launcher. Never resets the database or stops manual services.
set -euo pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_dir"
source "$project_dir/scripts/lib/capacity.sh"
if (( EUID == 0 )); then
  echo 'Запускайте скрипт обычным пользователем, без sudo.' >&2
  exit 1
fi
state_dir="$project_dir/work/dev"
mkdir -p "$state_dir/logs" "$state_dir/bin"
chmod 700 "$state_dir"
project_key="$(printf '%s' "$project_dir" | sha256sum | cut -c1-12)"
prefix="contest-dev-$(id -u)-$project_key"
services=(analyzers runner backend frontend)
new_services=()
unit() { printf '%s-%s.service' "$prefix" "$1"; }
active() { systemctl is-active --quiet "$(unit "$1")"; }
fail() { echo "Ошибка: $*" >&2; exit 1; }
socket_ready() { curl --silent --output /dev/null --max-time 1 --unix-socket "$1" http://localhost/; }
port_busy() { [[ -n "$(ss -H -ltn "sport = :$1")" ]]; }

valid_lan_ip() {
  local address=$1 octet
  [[ "$address" =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ ]] || return 1
  local parts
  IFS=. read -r -a parts <<< "$address"
  for octet in "${parts[@]}"; do
    [[ "$octet" == 0 || "$octet" != 0* ]] || return 1
    (( 10#$octet <= 255 )) || return 1
  done
  [[ "${parts[0]}" == 10 || "${parts[0]}.${parts[1]}" == 192.168 ||
     ( "${parts[0]}" == 172 && ${parts[1]} -ge 16 && ${parts[1]} -le 31 ) ]]
}

detect_lan_ip() {
  local interface address
  # VPN TUN interfaces often install a lower-metric "default dev ..." route.
  # Prefer the router route with an actual gateway, not the VPN default.
  interface=$(ip -4 route show default | awk '/ via / {for(i=1;i<NF;i++) if($i=="dev" && $(i+1) !~ /^(tun|tap|tailscale|wg|happ|xray)/) {print $(i+1); exit}}')
  if [[ -n "$interface" ]]; then
    address=$(ip -4 -o addr show dev "$interface" scope global | awk 'NR==1 {split($4,a,"/"); print a[1]}')
    if valid_lan_ip "$address"; then printf '%s' "$address"; return; fi
  fi
  # A router without internet may have no default route. Ignore virtual bridges.
  ip -4 -o addr show up scope global | awk '$2 !~ /^(lo|docker|br-|veth|virbr|tun|tap|tailscale|wg|happ|xray)/ {split($4,a,"/"); print a[1]}' |
    while read -r address; do
      if valid_lan_ip "$address"; then printf '%s' "$address"; break; fi
    done
}

rollback() {
  local result=$?
  trap - EXIT
  if (( result != 0 && ${#new_services[@]} > 0 )); then
    echo 'Запуск не завершён. Останавливаю только сервисы, запущенные этой командой.' >&2
    local name
    for ((i=${#new_services[@]}-1; i>=0; i--)); do
      name="${new_services[i]}"
      sudo -n systemctl stop "$(unit "$name")" || true
    done
    echo "Логи: $state_dir/logs" >&2
  fi
  exit "$result"
}

launch() {
  local name=$1 run_uid=$2 run_gid=$3 directory=$4
  shift 4
  if active "$name"; then echo "$name: уже запущен"; return; fi
  touch "$state_dir/logs/$name.log"
  chmod 600 "$state_dir/logs/$name.log"
  sudo -n systemctl reset-failed "$(unit "$name")" 2>/dev/null || true
  local options=(--quiet --collect --unit="$(unit "$name")"
    --uid="$run_uid" --gid="$run_gid" --working-directory="$directory"
    --setenv="PATH=$PATH"
    --property=KillMode=mixed --property=TimeoutStopSec=60
    --property="StandardOutput=append:$state_dir/logs/$name.log"
    --property="StandardError=append:$state_dir/logs/$name.log")
  if [[ "$name" == backend ]]; then
    options+=(--property="EnvironmentFile=$state_dir/backend.env")
  fi
  sudo -n systemd-run "${options[@]}" -- "$@"
  new_services+=("$name")
}

wait_service() {
  local name=$1
  shift
  for ((attempt=0; attempt<45; attempt++)); do
    if ! active "$name"; then
      tail -n 15 "$state_dir/logs/$name.log" >&2
      fail "$name завершился. См. ./scripts/dev.sh logs $name"
    fi
    if "$@"; then echo "$name: готов"; return; fi
    sleep 1
  done
  fail "$name не стал доступен за 45 секунд. См. ./scripts/dev.sh logs $name"
}

write_env() {
  local key=$1 value=$2
  [[ "$value" != *$'\n'* && "$value" != *$'\r'* ]] || fail "Перенос строки в $key"
  value=${value//\\/\\\\}
  value=${value//\"/\\\"}
  printf '%s="%s"\n' "$key" "$value" >> "$state_dir/backend.env"
}

case "${1:-}" in
  start)
    bind_host=127.0.0.1
    case "${2:-}" in
      '') [[ $# == 1 ]] || fail 'Использование: start [--lan]' ;;
      --lan)
        [[ $# == 2 ]] || fail 'Использование: start [--lan]'
        if [[ -n "${LAN_IP:-}" ]]; then bind_host=$LAN_IP
        else
          command -v ip >/dev/null || fail 'Нужна команда ip либо переменная LAN_IP.'
          bind_host=$(detect_lan_ip)
        fi
        valid_lan_ip "$bind_host" || fail 'Не найден IPv4 локальной сети. Укажите LAN_IP=192.168.1.100 ./scripts/dev.sh start --lan'
        ;;
      *) fail 'Использование: start [--lan]' ;;
    esac
    for dependency in systemctl systemd-run flock curl ss go npm sudo nproc; do
      command -v "$dependency" >/dev/null || fail "Не найдена команда $dependency"
    done
    [[ -d /run/systemd/system ]] || fail 'Для этого скрипта нужен Linux с systemd.'
    exec 9>"$state_dir/launcher.lock"
    flock -n 9 || fail 'Другая команда запуска/остановки уже работает.'
    if [[ -n "${ANALYZER_SESSIONS:-}" && ! "$ANALYZER_SESSIONS" =~ ^[1-6]$ ]]; then
      fail 'ANALYZER_SESSIONS должен быть числом от 1 до 6.'
    fi
    if active analyzers; then
      analyzer_sessions=2
      if [[ -f "$state_dir/analyzer-sessions" ]]; then read -r analyzer_sessions < "$state_dir/analyzer-sessions"; fi
      if [[ -n "${ANALYZER_SESSIONS:-}" && "$ANALYZER_SESSIONS" != "$analyzer_sessions" ]]; then
        fail 'Для смены числа анализаторов сначала выполните ./scripts/dev.sh stop.'
      fi
    else
      available_kb=$(awk '/^MemAvailable:/ {print $2}' /proc/meminfo)
      capacity=$(analyzer_capacity "$(nproc)" "$available_kb")
      (( capacity >= 1 )) || fail 'Недостаточно свободной памяти для анализатора вместе с исполнителями.'
      analyzer_sessions=${ANALYZER_SESSIONS:-$capacity}
      (( analyzer_sessions <= capacity )) || fail "Безопасный бюджет сейчас: не больше $capacity анализаторов."
    fi
    echo "Сессий анализаторов: $analyzer_sessions (по 1 CPU / 1 ГиБ, gVisor)."
    previous_host=127.0.0.1
    if [[ -f "$state_dir/frontend-host" ]]; then read -r previous_host < "$state_dir/frontend-host"; fi
    if active frontend && [[ "$previous_host" != "$bind_host" ]]; then
      fail 'Для смены адреса/режима сначала выполните ./scripts/dev.sh stop, затем start [--lan].'
    fi
    [[ -d frontend/node_modules ]] || fail 'Сначала выполните npm --prefix frontend ci'
    for pair in backend:8080 frontend:5173; do
      name=${pair%:*}; port=${pair#*:}
      if ! active "$name" && port_busy "$port"; then
        fail "Порт $port занят. Остановите вручную запущенный $name через Ctrl+C и повторите start."
      fi
    done
    for pair in analyzers:/run/contest-analyzers/lsp.sock runner:/run/contest-runner/run.sock; do
      name=${pair%%:*}; socket=${pair#*:}
      if ! active "$name" && socket_ready "$socket" 2>/dev/null; then
        fail "$name уже запущен вручную. Остановите его через Ctrl+C и повторите start."
      fi
    done
    sudo -v
    sudo -n /usr/bin/docker info >/dev/null
    for image in contest-gopls:local contest-pylsp:local contest-csharp-ls:local contest-run-go:local contest-run-python:local contest-run-csharp:local; do
      sudo -n /usr/bin/docker image inspect "$image" >/dev/null 2>&1 || fail "Нет образа $image. Выполните ./scripts/analyzers.sh build и ./scripts/runner.sh build"
    done
    trap rollback EXIT
    echo 'Собираю локальные сервисы…'
    go -C backend build -buildvcs=false -o "$state_dir/bin/analyzerd" ./cmd/analyzerd
    go -C backend build -buildvcs=false -o "$state_dir/bin/runnerd" ./cmd/runnerd
    go -C backend build -buildvcs=false -o "$state_dir/bin/backend" .
    echo 'Запускаю PostgreSQL, сохраняя существующую базу…'
    if sudo -n /usr/bin/docker container inspect contest-main-postgres-1 >/dev/null 2>&1; then
      sudo -n /usr/bin/docker start contest-main-postgres-1 >/dev/null
    else
      sudo -n env POSTGRES_PORT="${POSTGRES_PORT:-55432}" /usr/bin/docker compose up -d --pull never --no-build postgres
    fi
    database_ready=false
    for ((attempt=0; attempt<45; attempt++)); do
      if sudo -n /usr/bin/docker exec contest-main-postgres-1 pg_isready -U contest -d contest01 >/dev/null 2>&1; then database_ready=true; break; fi
      sleep 1
    done
    "$database_ready" || fail 'PostgreSQL не стал доступен за 45 секунд.'
    if ! active backend; then
      (umask 077; : > "$state_dir/backend.env")
      chmod 600 "$state_dir/backend.env"
      write_env DATABASE_URL "${DATABASE_URL:-postgres://contest:contest@127.0.0.1:${POSTGRES_PORT:-55432}/contest01?sslmode=disable}"
      write_env ADMIN_TOKEN "${ADMIN_TOKEN:-dev-admin-token}"
      write_env PORT 8080
      write_env LISTEN_HOST 127.0.0.1
      write_env ANALYZER_SOCKET /run/contest-analyzers/lsp.sock
      write_env RUNNER_SOCKET /run/contest-runner/run.sock
      write_env STATIC_DIR ''
    fi
    launch analyzers 0 0 "$project_dir" "$state_dir/bin/analyzerd" --gid "$(id -g)" --runtime runsc --max-sessions "$analyzer_sessions"
    wait_service analyzers socket_ready /run/contest-analyzers/lsp.sock
    printf '%s\n' "$analyzer_sessions" > "$state_dir/analyzer-sessions"
    launch runner 0 0 "$project_dir" "$state_dir/bin/runnerd" --gid "$(id -g)"
    wait_service runner socket_ready /run/contest-runner/run.sock
    launch backend "$(id -u)" "$(id -g)" "$project_dir/backend" "$state_dir/bin/backend"
    wait_service backend curl --fail --silent --output /dev/null --max-time 1 http://127.0.0.1:8080/health
    launch frontend "$(id -u)" "$(id -g)" "$project_dir/frontend" "$(command -v npm)" run dev -- --host "$bind_host" --port 5173 --strictPort
    wait_service frontend curl --fail --silent --output /dev/null --max-time 1 "http://$bind_host:5173/"
    printf '%s\n' "$bind_host" > "$state_dir/frontend-host"
    trap - EXIT
    if [[ "$bind_host" == 127.0.0.1 ]]; then echo 'Готово: http://localhost:5173'
    else echo "Адрес сайта для преподавателя и студентов: http://$bind_host:5173"; fi
    echo "Логи: $state_dir/logs"
    ;;
  stop)
    exec 9>"$state_dir/launcher.lock"
    flock -n 9 || fail 'Другая команда запуска/остановки уже работает.'
    sudo -v
    for name in frontend backend runner analyzers; do
      if systemctl show --property=LoadState --value "$(unit "$name")" 2>/dev/null | grep -qv '^not-found$'; then
        sudo -n systemctl stop "$(unit "$name")"
      fi
    done
    echo 'Сервисы остановлены. PostgreSQL оставлен работающим; данные сохранены.'
    ;;
  status)
    for name in "${services[@]}"; do
      state="$(systemctl is-active "$(unit "$name")" 2>/dev/null || true)"
      printf '%-12s %s\n' "$name" "${state:-inactive}"
    done
    if active analyzers && [[ -f "$state_dir/analyzer-sessions" ]]; then
      read -r analyzer_sessions < "$state_dir/analyzer-sessions"
      echo "Сессий анализаторов: $analyzer_sessions"
    fi
    if sudo -n /usr/bin/docker container inspect contest-main-postgres-1 --format 'PostgreSQL   {{.State.Status}}' 2>/dev/null; then :
    else echo 'PostgreSQL   нет сведений (проверка Docker требует sudo)'; fi
    if active frontend && [[ -f "$state_dir/frontend-host" ]]; then
      read -r bind_host < "$state_dir/frontend-host"
      echo "Адрес сайта: http://$bind_host:5173"
    fi
    ;;
  logs)
    name=${2:-}
    case "$name" in
      analyzers|runner|backend|frontend) tail -n 80 "$state_dir/logs/$name.log" ;;
      '') for name in "${services[@]}"; do echo "[$name]"; tail -n 15 "$state_dir/logs/$name.log" 2>/dev/null || true; done ;;
      *) fail 'Сервисы: analyzers, runner, backend, frontend' ;;
    esac
    ;;
  *) echo 'Использование: ./scripts/dev.sh start [--lan] | stop | status | logs [сервис]'; exit 2 ;;
esac
