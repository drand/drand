#!/bin/bash -x

dfinityPath="$HOME/prog/dfinity/crypto"
pkgDedisPath="$GOPATH/src/gopkg.in"
srcDedisPath="$GOPATH/src/github.com"
docker rm -f dfinity
docker run --rm -it --name dfinity -v $dfinityPath:/workspace -v $pkgDedisPath:/workspace/go/src/gopkg.in -v $srcDedisPath:/workspace/go/src/github.com dfinity/build 
