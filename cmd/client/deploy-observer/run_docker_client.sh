#!/bin/sh
set -x
docker run -it \
        -e DRAND_CLIENT_METRICS_ID="petar_test" \
        drandorg/observer-client:1.0
