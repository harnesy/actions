#!/usr/bin/env bash
# disk: free space on the disk that holds the project (harnsy action, bash).
# $1 max_used: fail when the disk is fuller than this, in percent.
set -euo pipefail

max_used=${1:-90}

echo "::step reading the disk of $(pwd)"
# df -P: one line per disk, the same columns on Linux and macOS; sizes in KiB.
read -r size_k used_k free_k used_pct < <(df -Pk . | awk 'NR == 2 { sub("%", "", $5); print $2, $3, $4, $5 }')

gb() { awk -v k="$1" 'BEGIN { printf "%.1f", k / 1048576 }'; }
free_gb=$(gb "$free_k")
used_gb=$(gb "$used_k")
size_gb=$(gb "$size_k")

# The result: one JSON object (contract/action-result.schema.json). used_gb
# is declared "of" size_gb: the dashboard shows it as used of size.
printf '{"summary":"%s GB free of %s GB (%s%% used)","numbers":{"free_gb":%s,"used_gb":%s,"size_gb":%s,"used_pct":%s}}\n' \
  "$free_gb" "$size_gb" "$used_pct" "$free_gb" "$used_gb" "$size_gb" "$used_pct" >"$ACTION_RESULT"

if ((used_pct > max_used)); then
  echo "::fail the disk is ${used_pct}% full, above ${max_used}%"
  exit 1
fi
