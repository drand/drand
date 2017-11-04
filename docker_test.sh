#!/bin/bash 
set -x
# This script spins off N drand containers and tries to verify any randomness
# produced.
# It's avery ad-hoc testing and there are probably better ways to do it but
# docker-compose had a "port being already taken" problem that I did not
# resolved...

## number of nodes
N=6
TMP=$(mktemp -d)
GROUPFILE="$TMP/group.toml"
IMG="dedis/drand"
DRAND_PATH="src/github.com/dedis/drand"
DOCKERFILE="$GOPATH/$DRAND_PATH/Dockerfile"
NET="drand"
SUBNET="192.168.0."
PORT="800"

source run_local.sh

build
run false

function checkSuccess() {
    if [ "$1" -eq 0 ]; then
        return
    else
        echo "TEST <$2>: FAILURE"
        cleanup
        exit 1
    fi
}

# wait for the node to actually do the DKG and run at least one beacon
sleep 3
#docker logs node1
rootFolder="$TMP/node1"
ret=0
# check if there are any signatures
ls "$rootFolder/beacons"| grep "sig" 
checkSuccess $? "any signature produced?"

# tail returns 0 in both cases...
sigFile=$(ls "$rootFolder/beacons"| grep "sig" | tail -n 1)

# check if there is the dist public key
distPublic="$rootFolder/dist_key.public"
ls "$rootFolder/dist_key.public"
checkSuccess $? "distributed public key file?"

# try to verify with it
#drand verify --distkey "$distPublic" "$rootFolder/beacons/$sigFile"
docker run --rm -v $distPublic:/group.key -v $rootFolder/beacons/$sigFile:/beacon.sig  \
        $IMG verify --distkey /group.key  /beacon.sig
checkSuccess $? "verify signature?"

echo "TESTS OK"
cleanup
