#!/usr/bin/env bash
set -euo pipefail
umask 077

source "$(dirname -- "${BASH_SOURCE[0]}")/common.sh"
dry=0 rollback=0
while [[ ${1:-} == --* ]]; do
    case "$1" in
        --dry-run) dry=1 ;;
        --rollback) rollback=1 ;;
        *) fail 'Usage: deploy.sh [--dry-run] [--rollback] USER@HOST /absolute/remote/dir' ;;
    esac
    shift
done
[[ $# == 2 ]] || fail 'Usage: deploy.sh [--dry-run] [--rollback] USER@HOST /absolute/remote/dir'
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
    rmdir -- "$temp"
    if [[ -n $stage ]]; then
        ssh "${ssh_args[@]}" "if test -d '$stage'; then rm -f -- '$stage'/release.tar.gz '$stage'/stack.tar.gz '$stage'/compose.yaml '$stage'/images.env '$stage'/deploy/apply-release.sh '$stage'/deploy/backup.sh '$stage'/deploy/common.sh '$stage'/deploy/proxy.conf '$stage'/deploy/restore.sh; rmdir -- '$stage/deploy' 2>/dev/null; rmdir -- '$stage'; fi" || true
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
tar -czf "$temp/stack.tar.gz" compose.yaml deploy/{apply-release,backup,common,restore}.sh deploy/proxy.conf

uploaded=$(ssh "${ssh_args[@]}" "mktemp -d '$remote/.incoming.XXXXXXXX'")
[[ $uploaded == "$remote/.incoming.${uploaded##*.}" && ${uploaded##*.} =~ ^[a-zA-Z0-9]{8}$ ]] || fail 'Unexpected remote staging path.'
stage=$uploaded
scp "${scp_args[@]}" "$temp/release.tar.gz" "$temp/stack.tar.gz" "$temp/images.env" "$target:$stage/"
ssh "${ssh_args[@]}" "tar -xzf '$stage/stack.tar.gz' -C '$stage' && bash '$stage/deploy/apply-release.sh' '$remote' '$tag' '$stage' '$backup_dir'"
