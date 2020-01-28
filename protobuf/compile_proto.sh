#!/usr/bin/env bash

# fail automatically as soon as an error is detected
#set -e
#
#curr=$(pwd)
#echo "Compilation of protobufs definitions to go files"
#echo
#find . -type d -print | tail -n +2 | while read dir; 
#do
#    echo " - compiling directory $dir"
#    protoc -I. \
#        -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
#        --go_out=plugins=grpc:$dir/ \
#        #--grpc-gateway_out=logtostderr=true:$GOPATH/src \
#        #--swagger_out=logtostderr=true:$GOPATH/src \
#        $dir/*.proto 
#done
#echo
#echo "Done!"

# fail automatically as soon as an error is detected
set -e
#set -x

curr=$(pwd)
echo "Compilation of protobufs definitions to go files"
echo
find . -type d -print | tail -n +2 | while read dir; 
do
    dd=$(echo $dir | sed "s|^\./||")
    echo " - compiling directory $dd"
    protoc -I. \
        -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
        --go_out=plugins=grpc:. \
        --grpc-gateway_out=logtostderr=true:. \
        --swagger_out=logtostderr=true:. \
        $dd/*.proto 
        sed -r -i 's:"crypto(.*):"github.com/drand/drand/protobuf/crypto\1:g' $dd/*go
done
echo
echo "Done!"
