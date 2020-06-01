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
        --url $DRAND_CLIENT_URL \
        --hash $DRAND_CLIENT_HASH \
        --relays $DRAND_CLIENT_RELAYS \
        --network $DRAND_CLIENT_NETWORK \
        --port $DRAND_CLIENT_PORT \
        --client-metrics-address $DRAND_CLIENT_METRICS_ADDRESS \
        --client-metrics-gateway $DRAND_CLIENT_METRICS_GATEWAY \
        --client-metrics-id $DRAND_CLIENT_METRICS_ID \
        --client-metrics-push-interval $DRAND_CLIENT_METRICS_PUSH_INTERVAL \
        $@
