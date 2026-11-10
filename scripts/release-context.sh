#!/usr/bin/env bash
# Package a release APK as the monitor-downloads build context used by the Dockerfile.
# Prints the APK version. Shared by the release workflow and the CI image check.
set -euo pipefail

if [[ $# != 2 ]]; then
    echo 'Usage: release-context.sh PATH_TO_APK CONTEXT_DIR' >&2
    exit 2
fi

apk=$1
context=$2
downloads="$context/downloads"
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

# Make the public files readable by the non-root runtime user.
umask 022
mkdir -p "$downloads"
install -m 0644 "$apk" "$downloads/"
published_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
bash "$script_dir/download-manifest.sh" "$apk" "$published_at" > "$downloads/latest.json"
jq -er .version "$downloads/latest.json"
