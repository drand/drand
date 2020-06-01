#!/bin/sh
set -x
docker run -it \
        -e DRAND_CLIENT_URL="http://pl-us.testnet.drand.sh" \
        -e DRAND_CLIENT_HASH="138a324aa6540f93d0dad002aa89454b1bec2b6e948682cde6bd4db40f4b7c9b" \
        -e DRAND_CLIENT_PORT="41333" \
        -e DRAND_CLIENT_RELAYS="/ip4/13.56.40.83/tcp/44544/p2p/12D3KooWDYnLRFGnMuNhV5zoeKp8TyAjKE8joW75N3zYdUDQFtUd" \
        -e DRAND_CLIENT_METRICS_ADDRESS="0.0.0.0:22333" \
        -e DRAND_CLIENT_METRICS_ID="petar_test" \
        drandorg/demo-client:1.0
