# build this dockerfile with context pointed at the root of the drand repo
FROM golang:1.14.2-buster AS builder
MAINTAINER Hector Sanjuan <hector@protocol.ai>

ARG version=unknown
ARG gitCommit

ENV GOPATH /go
ENV SRC_PATH $GOPATH/src/github.com/drand/drand/
ENV GOPROXY https://proxy.golang.org

ENV SUEXEC_VERSION v0.2
ENV TINI_VERSION v0.19.0
RUN set -x \
        && cd /tmp \
        && git clone https://github.com/ncopa/su-exec.git \
        && cd su-exec \
        && git checkout -q $SUEXEC_VERSION \
        && make \
        && cd /tmp \
        && wget -q -O tini https://github.com/krallin/tini/releases/download/$TINI_VERSION/tini \
        && chmod +x tini

# Get the TLS CA certificates, they're not provided by busybox.
RUN apt-get update && apt-get install -y ca-certificates

COPY go.* $SRC_PATH
WORKDIR $SRC_PATH
RUN go mod download

COPY . $SRC_PATH
RUN set -x && make build-client

FROM busybox:1-glibc
MAINTAINER Hector Sanjuan <hector@protocol.ai>

ENV GOPATH                 /go
ENV SRC_PATH               /go/src/github.com/drand/drand
ENV DRAND_CLIENT_HOME             /data/drand_client
# client command-line arguments
ENV DRAND_CLIENT_NETWORK   ""
ENV DRAND_CLIENT_HASH   ""
ENV DRAND_CLIENT_URL   ""
ENV DRAND_CLIENT_RELAYS   ""
ENV DRAND_CLIENT_PORT   "42777"
ENV DRAND_CLIENT_METRICS_ADDRESS   "0.0.0.0:32111"
ENV DRAND_CLIENT_METRICS_GATEWAY ""
ENV DRAND_CLIENT_METRICS_ID ""
ENV DRAND_CLIENT_METRICS_PUSH_INTERVAL 10

# expose peer host
EXPOSE 42777

# expose promethius API
EXPOSE 32111

COPY --from=builder $SRC_PATH/drand-client /usr/local/bin/drand-client
COPY --from=builder $SRC_PATH/cmd/drand-client/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY --from=builder /tmp/su-exec/su-exec /sbin/su-exec
COPY --from=builder /tmp/tini /sbin/tini
COPY --from=builder /etc/ssl/certs /etc/ssl/certs

RUN mkdir -p $DRAND_CLIENT_HOME && \
        addgroup -g 994 drand_client && \
        adduser -D -h $DRAND_CLIENT_HOME -u 996 -G drand_client drand_client && \
        chown drand_client:drand_client $DRAND_CLIENT_HOME

VOLUME $DRAND_CLIENT_HOME
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/entrypoint.sh"]

# Defaults for demo-client go here
CMD ["--watch", "--insecure"]
