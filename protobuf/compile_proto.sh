#!/usr/bin/env bash

# fail automatically as soon as an error is detected
set -e

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
