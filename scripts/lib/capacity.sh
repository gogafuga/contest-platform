#!/usr/bin/env bash
# Pure planner: arguments are logical CPUs and MemAvailable in KiB.
analyzer_capacity() {
  local cpus=$1 available=$2 reserve slots cpu_slots
  [[ "$cpus" =~ ^[1-9][0-9]*$ && "$available" =~ ^[0-9]+$ ]] || return 2
  # Keep 2 GiB for runner containers and at least 1 GiB / 25% for other work.
  reserve=$((available / 4))
  (( reserve >= 1048576 )) || reserve=1048576
  slots=$(((available - 2097152 - reserve) / 1048576))
  if (( slots < 1 )); then printf '0\n'; return; fi
  cpu_slots=$((cpus / 2))
  (( cpu_slots >= 1 )) || cpu_slots=1
  (( slots <= cpu_slots )) || slots=$cpu_slots
  (( slots <= 6 )) || slots=6
  printf '%s\n' "$slots"
}
