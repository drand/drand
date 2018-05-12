FROM golang:1.10.2-alpine

MAINTAINER Nicolas GAILLY <nicolas.gailly@epfl.ch>

RUN apk add --update --no-cache git && \
  rm -rf /tmp/* /var/cache/apk/*

COPY . "/go/src/github.com/dedis/drand"
WORKDIR "/go/src/github.com/dedis/drand"
RUN go install && rm -rf "/go/src/github.com/dedis/drand"
WORKDIR /
ENTRYPOINT ["drand"]

