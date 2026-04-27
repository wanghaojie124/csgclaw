#!/usr/bin/env bash
# Publish dist/* to a Gitee release (API v5). Requires GITEE_TOKEN and GITEE_REPO (owner/repo).
# Optional: GITEE_TARGET_BRANCH (default main) for new release target_commitish.
set -euo pipefail

if [ "$#" -lt 1 ]; then
  echo "usage: $0 <tag> [dist_dir]" >&2
  exit 1
fi

VERSION="$1"
DIST_DIR="${2:-dist}"
GITEE_REPO="${GITEE_REPO:?set GITEE_REPO to owner/repo on Gitee}"
GITEE_TOKEN="${GITEE_TOKEN:?set GITEE_TOKEN}"
GITEE_TARGET_BRANCH="${GITEE_TARGET_BRANCH:-main}"

owner="${GITEE_REPO%%/*}"
repo="${GITEE_REPO#*/}"
if [ "$owner" = "$GITEE_REPO" ] || [ -z "$repo" ]; then
  echo "GITEE_REPO must be owner/repo (got: ${GITEE_REPO})" >&2
  exit 1
fi

api_base="https://gitee.com/api/v5/repos/${owner}/${repo}"

json_get_top_level_number() {
  local key="${1:?missing json key}"
  if command -v python3 >/dev/null 2>&1; then
    python3 -c 'import json,sys
key=sys.argv[1]
try:
    data=json.loads(sys.stdin.read())
    value=data.get(key)
    if isinstance(value, int):
        print(value)
except Exception:
    pass
' "$key"
    return
  fi
  # Fallback parser for environments without python3.
  sed -n "s/^[[:space:]]*\"${key}\"[[:space:]]*:[[:space:]]*\\([0-9][0-9]*\\).*/\\1/p" | head -n 1
}

release_id_for_tag() {
  local url status body
  url="${api_base}/releases/tags/${VERSION}?access_token=${GITEE_TOKEN}"
  body="$(curl -sS -g -w '\n%{http_code}' "$url")"
  status="$(echo "$body" | tail -n 1)"
  body="$(echo "$body" | sed '$d')"
  if [ "$status" = "200" ]; then
    printf '%s' "$body" | json_get_top_level_number id
    return 0
  fi
  echo ""
}

create_release() {
  local out status body
  out="$(curl -sS -X POST "${api_base}/releases?access_token=${GITEE_TOKEN}" \
    --data-urlencode "tag_name=${VERSION}" \
    --data-urlencode "name=${VERSION}" \
    --data-urlencode "body=Release ${VERSION} (mirrored from GitHub Actions)." \
    --data-urlencode "target_commitish=${GITEE_TARGET_BRANCH}" \
    -w '\n%{http_code}')"
  status="$(echo "$out" | tail -n 1)"
  body="$(echo "$out" | sed '$d')"
  case "$status" in
    201)
      printf '%s' "$body"
      ;;
    *)
      echo "create Gitee release failed: HTTP ${status}" >&2
      printf '%s\n' "$body" >&2
      return 1
      ;;
  esac
}

upload_file() {
  local rid path name status out
  rid="$1"
  path="$2"
  name="$(basename "$path")"
  out="$(curl -sS -X POST "${api_base}/releases/${rid}/attach_files?access_token=${GITEE_TOKEN}" \
    -F "file=@${path};filename=${name}" -w '\n%{http_code}')"
  status="$(echo "$out" | tail -n 1)"
  if [ "$status" != "201" ] && [ "$status" != "200" ]; then
    echo "upload failed for ${name} (HTTP ${status})" >&2
    echo "$out" | sed '$d' >&2
    exit 1
  fi
}

if [ ! -d "$DIST_DIR" ]; then
  echo "dist directory not found: ${DIST_DIR}" >&2
  exit 1
fi

shopt -s nullglob
files=("${DIST_DIR}"/*)
if [ "${#files[@]}" -eq 0 ]; then
  echo "no files under ${DIST_DIR}" >&2
  exit 1
fi

rid="$(release_id_for_tag)"
if [ -z "$rid" ]; then
  resp=""
  if resp="$(create_release)"; then
    rid="$(printf '%s' "$resp" | json_get_top_level_number id)"
  fi
  if [ -z "$rid" ]; then
    rid="$(release_id_for_tag)"
  fi
  if [ -z "$rid" ]; then
    echo "failed to get or create Gitee release for ${VERSION}" >&2
    if [ -n "$resp" ]; then
      printf '%s\n' "$resp" >&2
    fi
    exit 1
  fi
fi

release_check="$(curl -sS -g -w '\n%{http_code}' "${api_base}/releases/${rid}?access_token=${GITEE_TOKEN}")"
release_check_status="$(echo "$release_check" | tail -n 1)"
if [ "$release_check_status" != "200" ]; then
  echo "resolved release id is invalid for ${GITEE_REPO} tag ${VERSION}: ${rid} (HTTP ${release_check_status})" >&2
  echo "$release_check" | sed '$d' >&2
  exit 1
fi
echo "using Gitee release id ${rid} for tag ${VERSION}"

for f in "${files[@]}"; do
  if [ ! -f "$f" ]; then
    continue
  fi
  echo "uploading $(basename "$f")"
  upload_file "$rid" "$f" >/dev/null
done

echo "Gitee release ${VERSION} updated (id=${rid})"
