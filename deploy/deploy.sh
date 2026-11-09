#!/usr/bin/env bash
set -euo pipefail
umask 077

source "$(dirname -- "${BASH_SOURCE[0]}")/common.sh"
dry=0 rollback=0 apk=''
while [[ ${1:-} == --* ]]; do
    case "$1" in
        --dry-run) dry=1 ;;
        --rollback) rollback=1 ;;
        --apk)
            [[ $# -gt 1 && -z $apk && -n $2 ]] || fail '--apk requires one APK path.'
            apk=$2
            shift
            ;;
        *) fail 'Usage: deploy.sh [--dry-run] [--rollback | --apk FILE] USER@HOST /absolute/remote/dir' ;;
    esac
    shift
done
[[ $# == 2 ]] || fail 'Usage: deploy.sh [--dry-run] [--rollback | --apk FILE] USER@HOST /absolute/remote/dir'
if [[ -n $apk ]]; then
    (( ! rollback )) || fail '--apk cannot be used with --rollback.'
    [[ -f $apk && ! -L $apk && -s $apk ]] || fail 'APK must be a nonempty regular file.'
    apk_name=$(basename -- "$apk")
    [[ $apk_name =~ ^vanillapay-monitor-(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.apk$ ]] ||
        fail 'APK filename must be vanillapay-monitor-MAJOR.MINOR.PATCH.apk.'
    apk="$(cd -- "$(dirname -- "$apk")" && pwd -P)/$apk_name"
fi
target=$1 remote=$2
port=${DEPLOY_SSH_PORT:-22} platform=${DEPLOY_PLATFORM:-linux/amd64} backup_dir=${DEPLOY_BACKUP_DIR:-}
# These values are embedded as quoted arguments in a remote shell command.
[[ $target =~ ^[a-zA-Z0-9_][a-zA-Z0-9_.-]*@[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]] || fail 'Use USER@HOST (DNS name or IPv4 address).'
[[ $remote =~ ^(/[a-zA-Z0-9_][a-zA-Z0-9_.-]*)+$ ]] || fail 'Use an absolute, non-root remote directory without spaces or dot-dot.'
[[ -z $backup_dir || $backup_dir =~ ^(/[a-zA-Z0-9_][a-zA-Z0-9_.-]*)+$ ]] || fail 'Invalid DEPLOY_BACKUP_DIR.'
[[ $port =~ ^[1-9][0-9]{0,4}$ && $port -le 65535 ]] || fail 'Invalid DEPLOY_SSH_PORT.'
[[ $platform == linux/amd64 || $platform == linux/arm64 ]] || fail 'DEPLOY_PLATFORM must be linux/amd64 or linux/arm64.'

ssh_args=(-o BatchMode=yes -p "$port" "$target")
scp_args=(-o BatchMode=yes -P "$port")
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
tag=$(git rev-parse --short=12 HEAD)
if (( dry )); then
    printf 'SSH preflight on %s (port %s): require %s/.env and Docker Compose.\n' "$target" "$port" "$remote"
    if (( rollback )); then
        printf 'Back up current data, then start the previous release (last good release if an upgrade failed).\n'
    else
        printf 'Build vanillapay:%s for %s. Pull pinned dependencies and save a compressed image archive.\n' "$tag" "$platform"
        printf 'SCP the images and Compose files to %s:%s. Back up any existing release, load images and start Compose offline.\n' "$target" "$remote"
        if [[ -n $apk ]]; then
            printf 'Compute SHA-256 and size, generate latest.json, and publish %s in this release.\n' "$apk_name"
        else
            printf 'Reuse the current release downloads if available.\n'
        fi
    fi
    printf 'Verify /health/ready. Show recent logs and exit nonzero if startup fails.\n'
    exit 0
fi

# Git Bash must pass the Linux paths below to Windows executables verbatim.
export MSYS2_ARG_CONV_EXCL='*'
ssh "${ssh_args[@]}" "test -f '$remote/.env' || { echo 'Missing remote .env' >&2; exit 1; }; docker compose version"
if (( rollback )); then
    ssh "${ssh_args[@]}" "cd '$remote' && if test -L current; then release=current; else release=pending; fi && bash \"\$release/deploy/apply-release.sh\" '$remote' --rollback '' '$backup_dir'"
    exit 0
fi

[[ -z $(git status --porcelain) ]] || fail 'Commit or remove workspace changes before building a release.'
temp=$(mktemp -d)
stage=''
cleanup() {
    rm -f -- "$temp/release.tar.gz" "$temp/stack.tar.gz" "$temp/images.env"
    if [[ -n $apk && -d $temp/downloads ]]; then
        rm -f -- "$temp/downloads/latest.json" "$temp/downloads/$apk_name"
        rmdir -- "$temp/downloads"
    fi
    rmdir -- "$temp"
    if [[ -n $stage ]]; then
        ssh "${ssh_args[@]}" "if test -d '$stage'; then rm -rf -- '$stage'; fi" || true
    fi
}
trap cleanup EXIT

# Compose contains the canonical pins. Docker save/load preserves tags but
# does not reliably restore registry digest references on an offline daemon.
refs=$(env -u APP_IMAGE_TAG -u POSTGRES_IMAGE -u PROXY_IMAGE \
    docker compose --env-file deploy/env.example -f compose.yaml config --images)
pg_image=$(grep -E '^postgres:[^@]+@sha256:[a-f0-9]{64}$' <<< "$refs")
proxy_image=$(grep -E '^nginx:[^@]+@sha256:[a-f0-9]{64}$' <<< "$refs")
pg_alias="vanillapay-postgres:sha256-${pg_image##*:}"
proxy_alias="vanillapay-proxy:sha256-${proxy_image##*:}"
docker pull --platform "$platform" "$pg_image"
docker pull --platform "$platform" "$proxy_image"
docker image tag "$pg_image" "$pg_alias"
docker image tag "$proxy_image" "$proxy_alias"
docker build --platform "$platform" -t "vanillapay:$tag" .
docker image save --platform "$platform" "vanillapay:$tag" "$pg_alias" "$proxy_alias" |
    gzip -1 > "$temp/release.tar.gz"
printf 'APP_IMAGE_TAG=%s\nPOSTGRES_IMAGE=%s\nPROXY_IMAGE=%s\n' "$tag" "$pg_alias" "$proxy_alias" > "$temp/images.env"
files=(compose.yaml deploy/{apply-release,backup,common,restore}.sh deploy/proxy.conf)
if [[ -n $apk ]]; then
    mkdir -m 755 -- "$temp/downloads"
    cp -- "$apk" "$temp/downloads/$apk_name"
    apk_copy="$temp/downloads/$apk_name"
    chmod 644 "$apk_copy"
    sha=$(sha256sum < "$apk_copy")
    size=$(wc -c < "$apk_copy")
    version=${apk_name#vanillapay-monitor-}
    printf '{"version":"%s","file":"%s","sha256":"%s","size":%d,"published_at":"%s"}\n' \
        "${version%.apk}" "$apk_name" "${sha%% *}" "$size" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$temp/downloads/latest.json"
    chmod 644 "$temp/downloads/latest.json"
    files+=(-C "$temp" downloads)
fi
tar -czf "$temp/stack.tar.gz" "${files[@]}"

uploaded=$(ssh "${ssh_args[@]}" "mktemp -d '$remote/.incoming.XXXXXXXX'")
[[ $uploaded == "$remote/.incoming.${uploaded##*.}" && ${uploaded##*.} =~ ^[a-zA-Z0-9]{8}$ ]] || fail 'Unexpected remote staging path.'
stage=$uploaded
scp "${scp_args[@]}" "$temp/release.tar.gz" "$temp/stack.tar.gz" "$temp/images.env" "$target:$stage/"
ssh "${ssh_args[@]}" "tar -xzf '$stage/stack.tar.gz' -C '$stage' && bash '$stage/deploy/apply-release.sh' '$remote' '$tag' '$stage' '$backup_dir'"
