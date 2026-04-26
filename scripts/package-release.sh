#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: $0 <goos> <goarch>" >&2
  exit 1
fi

GOOS_TARGET="$1"
GOARCH_TARGET="$2"
APP="${APP:-csgclaw}"
CMD_PATH="${CMD_PATH:-./cmd/${APP}}"
VERSION="${VERSION:-dev}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
DIST_DIR="${DIST_DIR:-dist}"
GOCACHE="${GOCACHE:-$(pwd)/.gocache}"
GO_BUILD_TAGS="${GO_BUILD_TAGS:-}"
VERSION_PKG="${VERSION_PKG:-csgclaw/internal/version}"
LDFLAGS="-X ${VERSION_PKG}.Version=${VERSION} -X ${VERSION_PKG}.Commit=${COMMIT} -X ${VERSION_PKG}.BuildTime=${BUILD_TIME}"
if [ "$APP" = "csgclaw-cli" ]; then
  LDFLAGS="-s -w ${LDFLAGS}"
fi

mkdir -p "$DIST_DIR"
"$(dirname "$0")/sync-agent-runtimes.sh"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

binary_name="$APP"
archive_ext="tar.gz"
if [ "$GOOS_TARGET" = "windows" ]; then
  binary_name="${APP}.exe"
  archive_ext="zip"
fi

if [ -n "$GO_BUILD_TAGS" ]; then
  env GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" GOCACHE="$GOCACHE" \
    go build -tags "$GO_BUILD_TAGS" -ldflags "${LDFLAGS}" -o "${tmpdir}/${binary_name}" "${CMD_PATH}"
else
  env GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" GOCACHE="$GOCACHE" \
    go build -ldflags "${LDFLAGS}" -o "${tmpdir}/${binary_name}" "${CMD_PATH}"
fi

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
