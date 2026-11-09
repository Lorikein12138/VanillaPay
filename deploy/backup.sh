#!/usr/bin/env bash
set -euo pipefail

source "$(dirname -- "${BASH_SOURCE[0]}")/common.sh"

backup_stack() (
    umask 077
    local destination=${BACKUP_DIR:-"$stack_dir/backups"} days=${BACKUP_RETENTION_DAYS:-14}
    local stage='' resume=0 app
    [[ $days =~ ^[1-9][0-9]{0,3}$ ]] || fail 'BACKUP_RETENTION_DAYS must be a positive integer (at most four digits).'
    mkdir -p -- "$destination"
    destination=$(cd -- "$destination" && pwd -P)

    cleanup() {
        local status=$?
        trap - EXIT
        if [[ -n $stage ]]; then
            rm -f -- "$stage/database.dump" "$stage/app-data.tar.gz" "$stage/SHA256SUMS"
            rmdir -- "$stage"
        fi
        if (( resume )); then
            compose start app proxy || status=1
        fi
        exit "$status"
    }
    trap cleanup EXIT
    stage=$(mktemp -d "$destination/.backup.XXXXXXXX")

    app=$(compose ps --status running -q app)
    if [[ -n $app ]]; then
        resume=1
        compose stop proxy app >&2
    fi
    compose up -d --wait --wait-timeout 90 --pull never --no-deps postgres >&2
    compose exec -T postgres pg_dump -U vanillapay -d vanillapay --format=custom > "$stage/database.dump"
    compose run -T --rm --no-deps --entrypoint tar app -C /var/lib/vanillapay -czf - . > "$stage/app-data.tar.gz"
    (cd -- "$stage" && sha256sum database.dump app-data.tar.gz > SHA256SUMS)
    local completed="$destination/vanillapay-$(date -u +%Y%m%dT%H%M%SZ)-${stage##*.}"
    mv -T -- "$stage" "$completed"
    stage=''
    find "$destination" -mindepth 1 -maxdepth 1 -type d \
        -name 'vanillapay-20??????T??????Z-????????' -mtime "+$((days - 1))" \
        -exec rm -rf -- {} +
    printf '%s\n' "$completed"
)

if [[ ${BASH_SOURCE[0]} == "$0" ]]; then
    [[ $# == 0 ]] || fail 'Usage: backup.sh (optional BACKUP_DIR and BACKUP_RETENTION_DAYS)'
    use_release "$(dirname -- "${BASH_SOURCE[0]}")/.."
    lock_stack
    use_release "$(dirname -- "${BASH_SOURCE[0]}")/.."
    backup_stack
fi
