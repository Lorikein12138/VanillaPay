#!/bin/sh
set -eu

add_loopback_address() {
    address="$1"
    if ! ip -o -4 address show dev lo | awk '{print $4}' | grep -Fxq "$address/32"; then
        ip address add "$address/32" dev lo
    fi
    route="$(ip route get "$address")"
    case "$route" in
        *"local $address"*" dev lo"*) ;;
        *) echo "fixture route is not local loopback for $address" >&2; return 1 ;;
    esac
}

add_loopback_address 93.184.216.34
add_loopback_address 93.184.216.35
add_loopback_address 93.184.216.36

exec /usr/local/bin/network-fixture "$@"
