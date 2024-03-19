FROM --platform=linux/amd64 golang:1.20.1-buster AS builder
MAINTAINER Hector Sanjuan <hector@protocol.ai>

ARG major=0
ARG minor=0
ARG patch=0
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

# Get the TLS CA certificates, they"re not provided by busybox.
RUN apt-get update && apt-get install -y ca-certificates

COPY go.* $SRC_PATH
WORKDIR $SRC_PATH
RUN go mod download

COPY . $SRC_PATH
RUN \
  # go install \
  # -mod=readonly \
  # -ldflags \
  # "-X github.com/drand/drand/common.COMMIT=$(git rev-parse HEAD) \
  # -X github.com/drand/drand/common.BUILDDATE=$(date -u +%d/%m/%Y@%H:%M:%S) \
  # -X github.com/drand/drand/cmd/drand-cli.buildDate=$(date -u +%d/%m/%Y@%H:%M:%S) \
  # -X github.com/drand/drand/cmd/drand-cli.gitCommit=$(git rev-parse HEAD)" \
  # ./cmd/... \
  # && \
  go install \
  -mod=readonly \
  -ldflags \
  "-X github.com/drand/drand/common.COMMIT=$(git rev-parse HEAD) \
  -X github.com/drand/drand/common.BUILDDATE=$(date -u +%d/%m/%Y@%H:%M:%S) \
  -X github.com/drand/drand/cmd/drand-cli.buildDate=$(date -u +%d/%m/%Y@%H:%M:%S) \
  -X github.com/drand/drand/cmd/drand-cli.gitCommit=$(git rev-parse HEAD)"

# FROM --platform=linux/amd64 busybox:1-glibc
FROM --platform=linux/amd64 debian
MAINTAINER Hector Sanjuan <hector@protocol.ai>

ENV GOPATH                 /go
ENV SRC_PATH               /go/src/github.com/drand/drand
ENV DRAND_HOME             /data/drand
ENV DRAND_PUBLIC_ADDRESS   ""

EXPOSE 8888
EXPOSE 4444

COPY --from=builder \
  $GOPATH/bin/drand \
  # $GOPATH/bin/relay \
  # $GOPATH/bin/client \
  # $GOPATH/bin/relay-s3 \
  # $GOPATH/bin/relay-gossip \
  /usr/local/bin/

COPY --from=builder $SRC_PATH/docker/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY --from=builder /tmp/su-exec/su-exec /sbin/su-exec
COPY --from=builder /tmp/tini /sbin/tini
COPY --from=builder /etc/ssl/certs /etc/ssl/certs

RUN mkdir -p $DRAND_HOME && \
  addgroup --gid 994 drand && \
  adduser --disabled-password --home $DRAND_HOME --uid 996 --ingroup drand drand && \
  chown drand:drand $DRAND_HOME

RUN apt-get update && apt-get install -y iproute2 #libc-bin
VOLUME $DRAND_HOME
# ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/entrypoint.sh"]
ENTRYPOINT ["/bin/sh"]
# CMD ["tc", "qdisc", "add", "dev", "eth0", "root", "netem", "delay", "100ms"]

# Defaults for drand go here
# CMD ["start", "--tls-disable", "--control 8888", "--private-listen 0.0.0.0:4444"]
