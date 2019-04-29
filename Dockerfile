FROM golang:1.12.4-alpine

MAINTAINER Nicolas GAILLY <nikkolasg@gmail.com>

RUN apk update && apk upgrade && \
    apk add --no-cache bash git openssh
COPY . "/go/src/github.com/dedis/drand"
WORKDIR "/go/src/github.com/dedis/drand"
# from https://dev.to/plutov/docker-and-go-modules-3kkn
ENV GO111MODULE=on

COPY go.mod .
COPY go.sum .
RUN go mod download

RUN go install && rm -rf "/go/src/github.com/dedis/drand"

WORKDIR /

ENTRYPOINT ["drand"]

