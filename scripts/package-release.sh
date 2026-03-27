#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: $0 <goos> <goarch>" >&2
  exit 1
fi

GOOS_TARGET="$1"
GOARCH_TARGET="$2"
APP="${APP:-csgclaw}"
VERSION="${VERSION:-dev}"
DIST_DIR="${DIST_DIR:-dist}"
GOCACHE="${GOCACHE:-$(pwd)/.gocache}"

mkdir -p "$DIST_DIR"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

binary_name="$APP"
archive_ext="tar.gz"
if [ "$GOOS_TARGET" = "windows" ]; then
  binary_name="${APP}.exe"
  archive_ext="zip"
fi

env GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" GOCACHE="$GOCACHE" \
  go build -o "${tmpdir}/${binary_name}" ./cmd/csgclaw

archive_base="${APP}_${VERSION}_${GOOS_TARGET}_${GOARCH_TARGET}"

if [ "$GOOS_TARGET" = "windows" ]; then
  archive_path="${DIST_DIR}/${archive_base}.zip"
  if command -v zip >/dev/null 2>&1; then
    (
      cd "$tmpdir"
      zip -q "${OLDPWD}/${archive_path}" "${binary_name}"
    )
  elif command -v powershell.exe >/dev/null 2>&1; then
    powershell.exe -NoLogo -NoProfile -Command \
      "Compress-Archive -Path '${tmpdir//\//\\/}\\${binary_name}' -DestinationPath '${PWD//\//\\/}\\${archive_path}' -Force" >/dev/null
  else
    echo "zip or powershell.exe is required to package Windows artifacts" >&2
    exit 1
  fi
else
  tar -C "$tmpdir" -czf "${DIST_DIR}/${archive_base}.tar.gz" "${binary_name}"
fi

echo "packaged ${DIST_DIR}/${archive_base}.${archive_ext}"
