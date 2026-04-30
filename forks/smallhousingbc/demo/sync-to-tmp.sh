#!/usr/bin/env bash
# Mirror the demo's source files to /tmp/shbc-demo so Claude Preview
# (which runs in a sandbox that can't read user-home directories) can
# serve them. Run this any time the source changes.
set -euo pipefail

SRC="$(cd "$(dirname "$0")" && pwd)"
DST="/tmp/shbc-demo"

mkdir -p "$DST"
rsync -a --delete \
  --exclude '.git' \
  --exclude 'sync-to-tmp.sh' \
  "$SRC/" "$DST/"

echo "synced $SRC → $DST"
echo "tip: launch with Claude Preview (port 4319) or python3 -m http.server 8765 from $DST"
