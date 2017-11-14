FROM alpine 

MAINTAINER Nicolas GAILLY <nicolas.gailly@epfl.ch>

ENV GOPATH=/go
ENV PATH=$PATH:$GOPATH/bin

RUN apk add --update --no-cache bash \
            git \
            make \
            clang \
            llvm \
            gmp \
            gmp-dev \
            libgmpxx  \
            libstdc++ \
            g++ \
            openssl \
            openssl-dev \ 
            go && \
            git clone https://github.com/dfinity/bn /bn && \
            cd /bn && \
            make install && make && \
            rm -rf /bn && mkdir -p /go/src && \
            rm -rf /var/cache/apk && \
            rm -rf /usr/share/man && \
            apk del git make clang llvm && \
            mkdir -p /go/src/github.com/dedis/drand 
COPY . "/go/src/github.com/dedis/drand"
WORKDIR "/go/src/github.com/dedis/drand"
RUN go install && rm -rf "/go/src/"

ENTRYPOINT ["drand"]

