FROM golang:1.14-alpine

MAINTAINER Nicolas GAILLY <nikkolasg@gmail.com>

COPY drand /
ENTRYPOINT ["/drand"]

