#!/bin/sh
set -x
demo-client \
        --url http://pl-us.testnet.drand.sh \
        --hash 138a324aa6540f93d0dad002aa89454b1bec2b6e948682cde6bd4db40f4b7c9b \
        --port 41333 \
        --relays /ip4/13.56.40.83/tcp/44544/p2p/12D3KooWDYnLRFGnMuNhV5zoeKp8TyAjKE8joW75N3zYdUDQFtUd \
        --client-metrics-address 0.0.0.0:22333 \
        --client-metrics-id petar_test \
        --watch

# XXX
#       multiple URLs
#       deploy telegraf with client
#       telegraf.conf: env=XXX, build=XXX


# deployments
#       https://github.com/drand/drand-infra#deployments
# push gateway address
#       influxDBv2 details:
#       endpoint is: https://us-west-2-1.aws.cloud2.influxdata.com
#       bucket is: observatory
#       token is:  IVfBSLqDzuXcp08K8JSktkx1u1h-HQwjsTO6NcwpfIR3EDSCw_iLSmsC9cblLghZfPjUASbosd-bwFYUN8poGA==
#       You can use telegraf with prometheus inputs section to post to it.
# deploy to AWS?
#       ???
# get gossip multiaddresses
#       dig _dnsaddr.pl-us.testnet.drand.sh TXT
# influxdb UI
#       https://us-west-2-1.aws.cloud2.influxdata.com/orgs/fe4cb48e8109f62b/data-explorer?bucket=observatory
