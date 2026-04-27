#!/usr/bin/env bash
set -euo pipefail

APP="${APP:-csgclaw}"
REPO="${REPO:-OpenCSGs/csgclaw}"
VERSION="${VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
LIB_DIR="${LIB_DIR:-$HOME/.local/lib/${APP}}"
BASE_URL="${BASE_URL:-https://github.com/${REPO}/releases/download}"
TMPDIR_INSTALL=""

cleanup() {
  if [ -n "${TMPDIR_INSTALL:-}" ] && [ -d "${TMPDIR_INSTALL:-}" ]; then
    rm -rf "$TMPDIR_INSTALL"
  fi
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

detect_os() {
  case "$(uname -s)" in
    Darwin) echo "darwin" ;;
    Linux) echo "linux" ;;
    *)
      echo "unsupported OS: $(uname -s)" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *)
      echo "unsupported architecture: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

ensure_supported_platform() {
  case "$1/$2" in
    darwin/arm64|linux/amd64) ;;
    *)
      echo "unsupported platform: $1/$2" >&2
      echo "prebuilt csgclaw binaries currently support macOS arm64 and Linux amd64 only" >&2
      exit 1
      ;;
  esac
}

resolve_latest_version() {
  local api_url tag
  api_url="https://api.github.com/repos/${REPO}/releases/latest"
  tag="$(curl -fsSL "$api_url" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  if [ -z "$tag" ]; then
    echo "failed to resolve latest release from ${api_url}" >&2
    exit 1
  fi
  echo "$tag"
}

ensure_install_dir() {
  mkdir -p "$INSTALL_DIR"
}

ensure_lib_dir() {
  mkdir -p "$LIB_DIR"
}

check_path_hint() {
  case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
      cat <<EOF

$INSTALL_DIR is not on your PATH.
Add this line to your shell profile:
  export PATH="$INSTALL_DIR:\$PATH"
EOF
      ;;
  esac
}

main() {
  need_cmd curl
  need_cmd tar
  need_cmd mktemp
  need_cmd install

  local os arch version archive_name download_url archive_path extracted_path bundle_path bundle_bin_path install_root
  os="$(detect_os)"
  arch="$(detect_arch)"
  ensure_supported_platform "$os" "$arch"
  version="$VERSION"
  if [ "$version" = "latest" ]; then
    version="$(resolve_latest_version)"
  fi

  archive_name="${APP}_${version}_${os}_${arch}.tar.gz"
  download_url="${BASE_URL}/${version}/${archive_name}"

  TMPDIR_INSTALL="$(mktemp -d)"
  trap cleanup EXIT
  archive_path="${TMPDIR_INSTALL}/${archive_name}"

  echo "Downloading ${download_url}"
  curl -fsSL "$download_url" -o "$archive_path"

  tar -xzf "$archive_path" -C "$TMPDIR_INSTALL"
  bundle_path="${TMPDIR_INSTALL}/${APP}"
  bundle_bin_path="${bundle_path}/bin/${APP}"

  ensure_install_dir
  ensure_lib_dir

  if [ -f "$bundle_bin_path" ]; then
    install_root="${LIB_DIR}/${version}"
    rm -rf "$install_root"
    mkdir -p "$install_root"
    cp -R "$bundle_path" "$install_root/"
    ln -sfn "${install_root}/${APP}/bin/${APP}" "${INSTALL_DIR}/${APP}"
    extracted_path="${install_root}/${APP}/bin/${APP}"
  else
    extracted_path="${TMPDIR_INSTALL}/${APP}"
    if [ ! -f "$extracted_path" ]; then
      echo "archive did not contain ${APP}" >&2
      exit 1
    fi
    install -m 0755 "$extracted_path" "${INSTALL_DIR}/${APP}"
    extracted_path="${INSTALL_DIR}/${APP}"
  fi

  cat <<EOF
Installed ${APP} ${version} to ${extracted_path}

Next steps:
  Choose one:
    ${APP} onboard --provider csghub-lite --models <model>
  or:
    ${APP} onboard --base-url <url> --api-key <key> --models <model>
  ${APP} serve
EOF
  check_path_hint
}

main "$@"
