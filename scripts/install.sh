#!/usr/bin/env bash
set -euo pipefail

APP="${APP:-csgclaw}"
REPO="${REPO:-OpenCSGs/csgclaw}"
# When mirror is gitee (or auto picks gitee), release assets may live under a different owner/repo on Gitee.
REPO_GITEE="${REPO_GITEE:-$REPO}"
VERSION="${VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
# CSGCLAW_MIRROR: auto (probe with curl, prefer GitHub when both are fine), github, or gitee.
CSGCLAW_MIRROR="${CSGCLAW_MIRROR:-auto}"
CSGCLAW_CONNECT_TIMEOUT="${CSGCLAW_CONNECT_TIMEOUT:-3}"
CSGCLAW_PROBE_MAX_TIME="${CSGCLAW_PROBE_MAX_TIME:-8}"
CSGCLAW_PROBE_GITHUB_URL="${CSGCLAW_PROBE_GITHUB_URL:-https://api.github.com/}"
CSGCLAW_PROBE_GITEE_URL="${CSGCLAW_PROBE_GITEE_URL:-https://gitee.com/}"
case "$CSGCLAW_MIRROR" in
  github|gitee|auto) ;;
  *)
    echo "unsupported CSGCLAW_MIRROR: ${CSGCLAW_MIRROR} (use auto, github, or gitee)" >&2
    exit 1
    ;;
esac
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

# HTTP probe (not ICMP ping): returns total time in seconds when the endpoint responds, empty on hard failure.
probe_http_time() {
  local url="$1" out time code
  out="$(curl -sS -o /dev/null -w '%{time_total} %{http_code}' \
    --connect-timeout "${CSGCLAW_CONNECT_TIMEOUT}" \
    --max-time "${CSGCLAW_PROBE_MAX_TIME}" \
    "$url" 2>/dev/null)" || {
    echo ""
    return
  }
  time="${out% *}"
  code="${out#* }"
  case "$code" in
    2?? | 3?? | 403 | 429) printf '%s' "$time" ;;
    *) echo "" ;;
  esac
}

# Pick github or gitee from probe latency; prefer GitHub on ties and when only one side works.
resolve_mirror_auto() {
  local gh_t gitee_t pick
  gh_t="$(probe_http_time "${CSGCLAW_PROBE_GITHUB_URL}")"
  gitee_t="$(probe_http_time "${CSGCLAW_PROBE_GITEE_URL}")"
  if [ -n "$gh_t" ] && [ -z "$gitee_t" ]; then
    echo github
    return
  fi
  if [ -z "$gh_t" ] && [ -n "$gitee_t" ]; then
    echo gitee
    return
  fi
  if [ -n "$gh_t" ] && [ -n "$gitee_t" ]; then
    pick="$(awk -v gh="$gh_t" -v ge="$gitee_t" 'BEGIN {
      if (gh + 0 <= ge + 0) print "github"; else print "gitee"
    }')"
    echo "$pick"
    return
  fi
  echo github
}

apply_base_url() {
  if [ -n "${BASE_URL:-}" ]; then
    return
  fi
  case "$CSGCLAW_MIRROR" in
    gitee) BASE_URL="https://gitee.com/${REPO_GITEE}/releases/download" ;;
    github) BASE_URL="https://github.com/${REPO}/releases/download" ;;
    *)
      echo "internal error: apply_base_url with CSGCLAW_MIRROR=${CSGCLAW_MIRROR}" >&2
      exit 1
      ;;
  esac
}

finalize_mirror_choice() {
  if [ "$CSGCLAW_MIRROR" = "auto" ]; then
    if [ -n "${BASE_URL:-}" ]; then
      case "$BASE_URL" in
        *gitee.com*) CSGCLAW_MIRROR=gitee ;;
        *) CSGCLAW_MIRROR=github ;;
      esac
    else
      CSGCLAW_MIRROR="$(resolve_mirror_auto)"
    fi
  fi
  apply_base_url
}

resolve_latest_version() {
  local api_url tag owner repo_name
  case "$CSGCLAW_MIRROR" in
    github)
      api_url="https://api.github.com/repos/${REPO}/releases/latest"
      ;;
    gitee)
      owner="${REPO_GITEE%%/*}"
      repo_name="${REPO_GITEE#*/}"
      api_url="https://gitee.com/api/v5/repos/${owner}/${repo_name}/releases/latest"
      if [ -n "${GITEE_ACCESS_TOKEN:-}" ]; then
        api_url="${api_url}?access_token=${GITEE_ACCESS_TOKEN}"
      fi
      ;;
  esac
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

  local os arch version archive_name download_url archive_path extracted_path
  finalize_mirror_choice
  echo "Using mirror: ${CSGCLAW_MIRROR} (${BASE_URL})"

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
  extracted_path="${TMPDIR_INSTALL}/${APP}"
  if [ ! -f "$extracted_path" ]; then
    echo "archive did not contain ${APP}" >&2
    exit 1
  fi

  ensure_install_dir
  install -m 0755 "$extracted_path" "${INSTALL_DIR}/${APP}"

  cat <<EOF
Installed ${APP} ${version} to ${INSTALL_DIR}/${APP}

Next steps:
  ${APP} onboard --base-url <url> --api-key <key> --model-id <model>
  ${APP} serve
EOF
  check_path_hint
}

main "$@"
