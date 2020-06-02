#!/bin/sh
set -x
docker run -it \
        -e DRAND_CLIENT_METRICS_ID="petar_testnet_observer_XXX" \
        drandorg/testnet-observer:1.0
