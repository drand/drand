FROM golang:1.12.4 as build

MAINTAINER Nicolas GAILLY <nikkolasg@gmail.com>

RUN apt-get update && \
    apt-get install -y  bash git openssh-server && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /go/src/github.com/dedis/drand

COPY . .

ENV GO111MODULE=on

RUN go install -mod=vendor


FROM gcr.io/distroless/base

COPY --from=build /go/bin/drand /

ENTRYPOINT ["/drand"]
