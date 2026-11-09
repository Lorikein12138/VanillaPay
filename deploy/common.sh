#!/usr/bin/env bash

fail() {
    printf '%s\n' "$*" >&2
    exit 1
}

use_release() {
    release_dir=$(cd -- "$1" && pwd -P)
    if [[ $(basename -- "$(dirname -- "$release_dir")") == .releases ]]; then
        stack_dir=$(cd -- "$release_dir/../.." && pwd -P)
    else
        stack_dir=$release_dir
    fi
    [[ -f "$stack_dir/.env" ]] || fail "Missing $stack_dir/.env"
}

compose() {
    local args=(--env-file "$stack_dir/.env" -f "$release_dir/compose.yaml")
    if [[ -f "$release_dir/images.env" ]]; then
        env -u APP_IMAGE_TAG -u POSTGRES_IMAGE -u PROXY_IMAGE \
            docker compose "${args[@]}" --env-file "$release_dir/images.env" "$@"
    else
        docker compose "${args[@]}" "$@"
    fi
}

lock_stack() {
    exec 9>"$stack_dir/.deploy.lock"
    flock -n 9 || fail 'Another deployment, backup or restore is running.'
}

start_stack() {
    local endpoint
    compose up -d --wait --wait-timeout 120 --pull never --no-build &&
        endpoint=$(compose port app 8080) &&
        curl --fail --silent --show-error --max-time 5 "http://$endpoint/health/ready" >/dev/null
}

recent_logs() {
    compose logs --tail 60 postgres migrate app proxy 2>&1 |
        sed -E 's/(Administrator initialization code: ).*/\1[redacted]/'
}
