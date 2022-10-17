#!/usr/bin/env bash

set -ex
stty -echoctl # hide ^C

err() {
   echo "$@" > /dev/stderr
}

pushd "$(git rev-parse --show-toplevel)"

bgPIDs=()
err "[+] Installing SIGINT trap ..."

shutdown() {
    err "[+] Stopping instances ..."
    for pid in "${bgPIDs[@]}"; do
        if ps -p "${pid}" > /dev/null
        then
            kill -9 "${pid}"
        fi
    done

    exit 0
}

trap 'shutdown' SIGINT

err "[+] You can now use SIGINT to terminate this script ..."

if [ "$1" == "-w" ]; then
    wait=1
fi

if [ -z "${DRAND_SHARE_SECRET}" ]; then
    err "Please set DRAND_SHARE_SECRET"
    exit 1
fi

err "[+] building drand ..."
go build

tmp=$(mktemp -d)
err "[+] base tmp folder: $tmp"

f=("$tmp/d1" "$tmp/d2" "$tmp/d3" "$tmp/d4" "$tmp/d5")

p=(5000 6000 7000 8000 9000)

for dir in "${f[@]}"; do
    mkdir -p "$dir"
done

err "[+] generating keys"
for i in "${!f[@]}"; do
    ./drand generate-keypair --tls-disable --folder "${f[${i}]}" 127.0.0.1:$((p["${i}"]+1))
done

err "[+ ] running drand daemons..."
for i in "${!f[@]}"; do
    ./drand --verbose start --tls-disable --control "${p[${i}]}" --private-listen 127.0.0.1:$((p["${i}"]+1)) --metrics $((p["${i}"]+2)) --folder "${f[${i}]}" & # > $tmp/log1 2>&1 &
    bgPIDs+=("$!")
done

sleep 0.1

err "[+] Starting initial dkg ..."

if [ -n "$wait" ]; then
    err "About to start initial dkg, hit ENTER to start leader"
    read -r
fi

./drand share --control "${p[0]}" --tls-disable --leader --id default --period 5s --nodes ${#f[@]} --threshold ${#f[@]} &
bgPIDs+=("$!")

for i in $(seq 1 $((${#f[@]}-1))); do
    if [ -n "$wait" ]; then
        err "About to share node $i. Hit ENTER to continue"
        read -r
    fi

    ./drand share --control "${p[${i}]}" --tls-disable --connect 127.0.0.1:$((p[0]+1))&
    bgPIDs+=("$!")
done

err "[+] Waiting 60s for the network to stabilize"
sleep 60

err "[+] Starting reshare"

if [ -n "$wait" ]; then
    err "About to start reshare, hit ENTER to start leader"
    read -r
fi

err -------------------------------------------------------------------------

./drand share --control "${p[0]}" --tls-disable --id default --transition --leader --nodes ${#f[@]} --threshold ${#f[@]} &
bgPIDs+=("$!")

for i in $(seq 1 $((${#f[@]}-1))); do
    if [ -n "$wait" ]; then
        err "About to reshare node $i. Hit ENTER to continue"
        read -r
    fi

err -------------------------------------------------------------------------
    ./drand share --control "${p[${i}]}" --tls-disable --id default --transition --connect 127.0.0.1:$((p[0]+1))&
    bgPIDs+=("$!")
done

popd

err "[+] done"

err "[+] Waiting for SIGINT to terminate..."

for pid in "${bgPIDs[@]}"; do
    if ps -p "${pid}" > /dev/null
    then
        wait "${pid}"
    fi
done

