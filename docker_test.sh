#!/bin/bash 
# set -x
# This script spins off N drand containers and tries to verify any randomness
# produced.
# It's avery ad-hoc testing and there are probably better ways to do it but
# docker-compose had a "port being already taken" problem that I did not
# resolved...

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
echo "[+] Waiting for beacon randomness protocol to generate a few beacons..."
sleep 5
#docker logs node1
rootFolder="$TMP/node1"

# check if there is the dist public key
distPublic="$rootFolder/groups/dist_key.public"
ls $distPublic > /dev/null 2> /dev/null
checkSuccess $? "distributed public key file?"

# try to verify with it
drandPublic="/dist_public.toml"
drandVol="$distPublic:$drandPublic"
drandArgs=("--debug" "fetch" "--public" $drandPublic "${addresses[1]}")
docker run --rm --net $NET --ip ${SUBNET}10 -v "$drandVol" $IMG "${drandArgs[@]}" 
checkSuccess $? "verify signature?"

echo "TESTS OK"
cleanup
