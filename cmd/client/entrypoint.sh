#!bin/sh

set -e
user=drand_client

if [ -n "$DOCKER_DEBUG" ]; then
   set -x
fi

if [ `id -u` -eq 0 ]; then
    echo "Changing user to $user"
    # ensure directories are writable
    su-exec "$user" test -w "${DRAND_CLIENT_HOME}" || chown -R -- "$user" "${DRAND_CLIENT_HOME}"
    exec su-exec "$user" "$0" $@
fi

exec drand-client \
        --url http://pl-us.testnet.drand.sh \
        --hash 138a324aa6540f93d0dad002aa89454b1bec2b6e948682cde6bd4db40f4b7c9b \
        --port 42777 \
        --relays /ip4/13.56.40.83/tcp/44544/p2p/12D3KooWDYnLRFGnMuNhV5zoeKp8TyAjKE8joW75N3zYdUDQFtUd \
        --client-metrics-address 0.0.0.0:32111 \
        --client-metrics-id $DRAND_CLIENT_METRICS_ID \
        $@
