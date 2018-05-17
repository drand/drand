FROM golang:1.10.2-alpine

MAINTAINER Nicolas GAILLY <nicolas.gailly@epfl.ch>

COPY . "/go/src/github.com/dedis/drand"
WORKDIR "/go/src/github.com/dedis/drand"
RUN go install && rm -rf "/go/src/github.com/dedis/drand"
WORKDIR /
ENTRYPOINT ["drand"]

