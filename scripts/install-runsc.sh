#!/usr/bin/env bash
# Installs the vendor runtime and registers it; does not restart Docker automatically.
set -euo pipefail
project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
case "$(uname -m)" in x86_64) runsc_arch=x86_64;; aarch64) runsc_arch=aarch64;; *) echo 'Unsupported architecture' >&2; exit 1;; esac
mkdir -p "$project_dir/work"
task_tmp="$(mktemp -d "$project_dir/work/runsc-install.XXXXXX")"
trap 'rm -rf "$task_tmp"' EXIT
cd "$task_tmp"
release_url="https://storage.googleapis.com/gvisor/releases/release/latest/$runsc_arch"
archive=gvisor.tar.bz2
curl --fail --location --connect-timeout 15 --max-time 600 --retry 2 --proto '=https' --tlsv1.2 "$release_url/$archive" -o "$archive"
curl --fail --location --connect-timeout 15 --max-time 60 --retry 2 --proto '=https' --tlsv1.2 "$release_url/$archive.sha512" -o "$archive.sha512"
sha512sum --check "$archive.sha512"
mkdir extracted
tar --extract --bzip2 --file "$archive" --directory extracted --no-same-owner --no-same-permissions
# A restrictive umask must not prevent runsc sidecars from re-executing as nobody.
chmod -R a+rX extracted
test -x extracted/runsc
test -d extracted/gvisor-bin
./extracted/runsc --version
# Keep runsc and its gvisor-bin sidecars together (required by current releases).
sudo install -d -m 0755 /usr/local/bin
sudo cp -R --no-preserve=ownership extracted/. /usr/local/bin/
sudo chown -R root:root /usr/local/bin/gvisor-bin
sudo chown root:root /usr/local/bin/runsc /usr/local/bin/containerd-shim-runsc-v1
if sudo test -f /etc/docker/daemon.json; then
 sudo cp -p /etc/docker/daemon.json "/etc/docker/daemon.json.before-gvisor-$(date +%Y%m%d-%H%M%S)"
fi
sudo /usr/local/bin/runsc install
printf '\ngVisor установлен и зарегистрирован. Активируйте runtime без остановки контейнеров:\n'
printf '  sudo systemctl reload docker\n'
printf '  sudo /usr/bin/docker info --format \047{{json .Runtimes}}\047\n'
printf 'Установщик сам Docker не перезагружал.\n'
