#!/usr/bin/env bash
set -euo pipefail
umask 077

source "$(dirname -- "${BASH_SOURCE[0]}")/backup.sh"
stack_dir=$(cd -- "$1" && pwd -P)
[[ -f "$stack_dir/.env" ]] || fail "Missing $stack_dir/.env"
lock_stack
cd -- "$stack_dir"
export BACKUP_DIR=${4:-${BACKUP_DIR:-"$stack_dir/backups"}}

shared_apks="$stack_dir/downloads/apks"

check_apk() {
    local source=$1 name=${1##*/}
    [[ $name =~ ^vanillapay-monitor-(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.apk$ &&
       -f $source && ! -L $source && -s $source ]] || fail "Invalid APK: $name"
    if [[ -e $shared_apks/$name || -L $shared_apks/$name ]]; then
        [[ -f $shared_apks/$name && ! -L $shared_apks/$name ]] && cmp -s -- "$source" "$shared_apks/$name" ||
            fail "APK $name already exists with different bytes or is not a regular file. Increment versionName."
    fi
}

publish_apk() {
    local source=$1 name=${1##*/} staged
    check_apk "$source"
    [[ ! -e $shared_apks/$name ]] || return 0
    staged=$(mktemp "$shared_apks/.apk.XXXXXXXX")
    if ! cp -- "$source" "$staged" || ! chmod 644 "$staged" || ! ln -T -- "$staged" "$shared_apks/$name"; then
        rm -f -- "$staged"
        fail "Could not append APK $name."
    fi
    rm -- "$staged"
}

mkdir -p -- "$shared_apks"
chmod 755 "$stack_dir/downloads" "$shared_apks"
# Import any files staged by the older release layout before using the shared mount.
for legacy in .releases/*/downloads/*.apk; do
    [[ -e $legacy || -L $legacy ]] || continue
    publish_apk "$legacy"
done

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
    apk=''
    if [[ -d $incoming/downloads ]]; then
        apk_files=("$incoming"/downloads/*.apk)
        [[ ${#apk_files[@]} == 1 && -f $incoming/downloads/latest.json && ! -L $incoming/downloads/latest.json ]] ||
            fail 'Expected one APK and its manifest.'
        apk=${apk_files[0]}
        check_apk "$apk"
        if [[ -e $target ]]; then
            grep -Fq -- "\"file\":\"${apk##*/}\"" "$target/downloads/latest.json" ||
                fail 'This Git tag already exists without this APK. Publish from a new commit.'
        fi
    fi
    if [[ -L current ]]; then
        use_release current
        backup_stack
    fi
    if [[ ! -e $target ]]; then
        docker image load -i "$incoming/release.tar.gz"
        mkdir -p -- "$incoming/downloads"
        chmod 755 "$incoming/downloads"
        if [[ -n $apk ]]; then
            publish_apk "$apk"
            rm -- "$apk"
        elif [[ -f current/downloads/latest.json ]]; then
            cp -- current/downloads/latest.json "$incoming/downloads/latest.json"
        fi
        if [[ -f $incoming/downloads/latest.json ]]; then
            chmod 644 "$incoming/downloads/latest.json"
        fi
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
