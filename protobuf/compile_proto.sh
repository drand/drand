#!/usr/bin/env bash

# fail automatically as soon as an error is detected
set -e

curr=$(pwd)
echo "Compilation of protobufs definitions to go files"
echo
find . -type d -print | tail -n +2 | while read dir; 
do
    echo " - compiling directory $dir"
    protoc -I. \
        -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
        --go_out=plugins=grpc:$GOPATH/src \
        --grpc-gateway_out=logtostderr=true:$GOPATH/src \
        --swagger_out=logtostderr=true:$GOPATH/src \
        $dir/*.proto 
done
echo
echo "Done!"
