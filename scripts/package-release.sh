#!/bin/bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SOURCE_DIR="${RELEASE_SOURCE_DIR:-${ROOT_DIR}/artifacts}"
PUBLISH_DIR="${ROOT_DIR}/build/publish"
VERSION="${BUILD_VERSION:?BUILD_VERSION is required}"
BASE_URL="${ARTIFACT_BASE_URL:?ARTIFACT_BASE_URL is required}"
COMMIT="$(git -C "${ROOT_DIR}" rev-parse HEAD)"
PUBLISHED_AT="$(date -u -d "@$(git -C "${ROOT_DIR}" show -s --format=%ct HEAD)" +%Y-%m-%dT%H:%M:%SZ)"

specs=(
  "windows|amd64|app|binary|signal_desktop-${VERSION}-windows-amd64.exe"
  "windows|amd64|launcher|binary|beagle-signal.launcher-${VERSION}-windows-amd64.exe"
  "darwin|arm64|app|zip|signal_desktop-${VERSION}-darwin-arm64.zip"
  "darwin|arm64|launcher|binary|beagle-signal.launcher-${VERSION}-darwin-arm64"
  "darwin|amd64|app|zip|signal_desktop-${VERSION}-darwin-amd64.zip"
  "darwin|amd64|launcher|binary|beagle-signal.launcher-${VERSION}-darwin-amd64"
  "linux|amd64|app|binary|signal_desktop-${VERSION}-linux-amd64"
  "linux|amd64|launcher|binary|beagle-signal.launcher-${VERSION}-linux-amd64"
)

rm -rf "${PUBLISH_DIR}"
mkdir -p "${PUBLISH_DIR}/updater/releases/desktop"
artifacts='[]'
for spec in "${specs[@]}"; do
    IFS='|' read -r os arch role package_type filename <<< "${spec}"
    source="${SOURCE_DIR}/${filename}"
    test -f "${source}"
    target="${PUBLISH_DIR}/desktop/artifacts/${os}/${arch}/${role}/${VERSION}/${filename}"
    mkdir -p "$(dirname "${target}")"
    cp "${source}" "${target}"
    item="$(jq -n --arg os "${os}" --arg arch "${arch}" --arg role "${role}" --arg type "${package_type}" \
      --arg filename "${filename}" --arg url "${BASE_URL}/desktop/artifacts/${os}/${arch}/${role}/${VERSION}/${filename}" \
      --arg sha "$(sha256sum "${source}" | awk '{print $1}')" --argjson size "$(stat -c %s "${source}")" \
      '{os:$os,arch:$arch,role:$role,package_type:$type,filename:$filename,download_url:$url,size:$size,sha256:$sha}')"
    artifacts="$(jq -c --argjson item "${item}" '. + [$item]' <<< "${artifacts}")"
done

jq -n --arg published_at "${PUBLISHED_AT}" --arg commit_date "${PUBLISHED_AT}" --arg version "${VERSION}" --arg commit "${COMMIT}" --argjson artifacts "${artifacts}" \
  '{schema_version:1,published_at:$published_at,release:{component:"desktop",version:$version,commit_id:$commit,commit_date:$commit_date,channel:"stable",release_notes:("Desktop build "+$commit),min_supported_version:""},artifacts:$artifacts}' \
  > "${PUBLISH_DIR}/updater/releases/desktop/latest.json"
jq -n --arg generated_at "${PUBLISHED_AT}" --arg base "${BASE_URL}" \
  '{schema_version:1,generated_at:$generated_at,manifests:[$base+"/updater/releases/agent/latest.json",$base+"/updater/releases/endpoint/latest.json",$base+"/updater/releases/desktop/latest.json"]}' \
  > "${PUBLISH_DIR}/updater/catalog.json"
