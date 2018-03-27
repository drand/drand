#!/usr/bin/env bash

curr=$(pwd)
echo "Compilation of protobufs definitions to go files"
echo
find . -type d -print | tail -n +2 | while read dir; 
do
    cd "$dir"
    echo " - compiling directory $dir"
    protoc --proto_path="$curr" -I . *.proto --go_out=plugins=grpc:. 
    cd "$curr"
done
echo
echo "Done!"
