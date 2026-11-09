#!/usr/bin/env bash
set -euo pipefail

source "$(dirname -- "${BASH_SOURCE[0]}")/common.sh"
[[ $# == 2 && $1 == --confirm ]] || fail 'Usage: restore.sh --confirm BACKUP_DIRECTORY'
backup=$(cd -- "$2" && pwd -P)
use_release "$(dirname -- "${BASH_SOURCE[0]}")/.."
lock_stack
use_release "$(dirname -- "${BASH_SOURCE[0]}")/.."

(
    cd -- "$backup"
    sha256sum database.dump app-data.tar.gz | cmp -s SHA256SUMS - ||
        fail 'Backup checksums do not match.'
    tar -tzf app-data.tar.gz >/dev/null
)
compose stop proxy app
compose up -d --wait --wait-timeout 90 --pull never --no-deps postgres

# Load into a staging database so a bad dump leaves the original DB intact.
compose exec -T postgres dropdb -U vanillapay --if-exists vanillapay_restore
compose exec -T postgres createdb -U vanillapay vanillapay_restore
compose exec -T postgres pg_restore -U vanillapay -d vanillapay_restore \
    --exit-on-error --single-transaction < "$backup/database.dump"

compose run -T --rm --no-deps --user 0 --entrypoint sh app -ec '
    cd /var/lib/vanillapay
    find . -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
    tar --numeric-owner -xzf -
' < "$backup/app-data.tar.gz"
compose exec -T postgres dropdb -U vanillapay --force vanillapay
compose exec -T postgres psql -X -v ON_ERROR_STOP=1 -U vanillapay -d postgres \
    -c 'ALTER DATABASE vanillapay_restore RENAME TO vanillapay'

if ! start_stack; then
    recent_logs
    fail 'Restore completed, but the stack did not become ready.'
fi
