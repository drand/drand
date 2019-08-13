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

# wait for the node to actually do the DKG and run at least one beacon
echo "[+] Waiting for beacon randomness protocol to generate a few beacons..."
sleep 5
#docker logs node1
rootFolder="$TMP/node1"

# check if there is the dist public key
distPublic="$rootFolder/groups/dist_key.public"
ls $distPublic
checkSuccess $? "distributed public key file?"

# try to verify with it
echo "[+] Verifying fetching public and private randomness"
fetchTest 2 true
#drandPublic="/dist_public.toml"
#drandVol="$distPublic:$drandPublic:z"
#drandArgs=("--debug" "fetch" "public" "--insecure" "--public" $drandPublic "${addresses[1]}")
#docker run --rm --net $NET --ip ${SUBNET}10 -v "$drandVol" $IMG "${drandArgs[@]}" 
#checkSuccess $? "verify signature?"

#echo "[+] Verifying fetching private randomness"
#serverId="/key/drand_id.public"
#drandVol="$rootFolder$serverId:$serverId:z"
#drandArgs=("--debug" "fetch" "private" "--insecure" $serverId)
#docker run --rm --net $NET --ip ${SUBNET}11 -v "$drandVol" $IMG "${drandArgs[@]}"
#checkSuccess $? "verify randomness encryption"

echo "TESTS OK"
cleanup
exit 0
