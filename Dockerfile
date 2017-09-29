FROM dedis/drand:bn

MAINTAINER Nicolas GAILLY <nicolas.gailly@epfl.ch>

RUN mkdir -p /go/src/github.com/dedis/drand
COPY . "/go/src/github.com/dedis/drand"
WORKDIR "/go/src/github.com/dedis/drand"
RUN go install 
WORKDIR /
RUN rm -rf "/go/src/"

ENTRYPOINT ["drand"]

