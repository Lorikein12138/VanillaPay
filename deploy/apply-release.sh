#!/usr/bin/env bash
set -euo pipefail
umask 077

source "$(dirname -- "${BASH_SOURCE[0]}")/backup.sh"
stack_dir=$(cd -- "$1" && pwd -P)
[[ -f "$stack_dir/.env" ]] || fail "Missing $stack_dir/.env"
lock_stack
cd -- "$stack_dir"
export BACKUP_DIR=${4:-${BACKUP_DIR:-"$stack_dir/backups"}}

if [[ $2 == --rollback ]]; then
    if [[ -L pending ]]; then
        use_release pending
        if [[ ! -L current ]]; then
            compose stop proxy app
            rm -- pending
            printf '%s\n' 'Stopped the failed initial release. Database and media are retained.'
            exit 0
        fi
        target=$(readlink current)
    else
        target=$(readlink previous) || fail 'No previous release.'
        use_release current
    fi
    backup_stack
else
    tag=$2
    [[ $tag =~ ^[a-f0-9]{7,40}$ ]] || fail 'Invalid release tag.'
    target=".releases/$tag"
    [[ ! -L pending ]] || fail 'An unfinished release exists. Roll back before deploying another release.'
    incoming=$3
    if [[ -L current ]]; then
        use_release current
        backup_stack
    fi
    if [[ ! -e $target ]]; then
        docker image load -i "$incoming/release.tar.gz"
        rm -- "$incoming/release.tar.gz" "$incoming/stack.tar.gz"
        chmod 755 "$incoming"/deploy/{apply-release,backup,common,restore}.sh
        mkdir -p -- .releases
        mv -T -- "$incoming" "$target"
    fi
fi

ln -sfn -- "$target" pending
use_release "$target"
if ! start_stack; then
    recent_logs
    fail 'Stack failed to become ready. For an existing stack, run deploy.sh --rollback.'
fi

if [[ -L current && $(readlink current) != "$target" ]]; then
    ln -sfn -- "$(readlink current)" previous
fi
mv -Tf -- pending current
printf 'Ready: %s\n' "${target##*/}"
