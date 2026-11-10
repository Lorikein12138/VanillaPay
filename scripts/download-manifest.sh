#!/usr/bin/env bash
set -euo pipefail

if [[ $# != 2 ]]; then
    echo 'Usage: download-manifest.sh PATH_TO_APK PUBLISHED_AT_UTC' >&2
    exit 2
fi

apk=$1
published_at=$2
file=${apk##*/}
if [[ ! $file =~ ^vanillapay-monitor-((0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*))\.apk$ || ! -f $apk || ! -s $apk ]]; then
    echo 'Expected a nonempty vanillapay-monitor-X.Y.Z.apk file' >&2
    exit 2
fi
version=${BASH_REMATCH[1]}
if [[ ! $published_at =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]] ||
   [[ $(date -u -d "$published_at" +%Y-%m-%dT%H:%M:%SZ) != "$published_at" ]]; then
    echo 'Expected an RFC3339 UTC publication date' >&2
    exit 2
fi

hash=$(sha256sum < "$apk")
size=$(wc -c < "$apk")
printf '{"version":"%s","file":"%s","sha256":"%s","size":%d,"published_at":"%s"}\n' \
    "$version" "$file" "${hash%% *}" "$size" "$published_at"
