FROM golang:alpine AS builder

ARG version=unknown
ARG gitCommit

WORKDIR /go/src/drand
COPY . .
ENV CGO_ENABLED 0
RUN go build -ldflags "-X main.version=${version} -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=${gitCommit}" -o /go/src/drand/drand
RUN chmod a+x /go/src/drand/drand

FROM scratch
MAINTAINER Nicolas GAILLY <nikkolasg@gmail.com>
ENV USER /
COPY --from=builder /go/src/drand/drand /drand
CMD ["/drand"]
ENTRYPOINT ["/drand"]

