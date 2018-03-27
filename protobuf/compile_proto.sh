#!/usr/bin/env bash

curr=$(pwd)
echo "Compilation of protobufs definitions to go files"
echo
find . -type d -print | tail -n +2 | while read dir; 
do
    echo " - compiling directory $dir"
    protoc -I. $dir/*.proto --go_out=plugins=grpc:$GOPATH/src
done
echo
echo "Done!"
