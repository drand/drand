#!/bin/bash

# set -x
# This script contains two parts.
# The first part is meant as a library, declaring the variables and functions to spins off drand containers
# The second part is triggered when this script is actually ran, and not
# sourced. This part calls the function to setup the drand containers and run
# them. It produces produce randomness in a temporary folder..
#
# NOTE: Using docker compose should give a higher degree of flexibility and
# composability. However I had trouble with spawning the containers too fast and
# having weird binding errors: port already in use. I rolled back to simple
# docker scripting. One of these, one should try to do it in docker-compose.
## number of nodes

N=6
BASE="/tmp/drand"
if [ ! -d "$BASE" ]; then
    mkdir -m 740 $BASE
fi
unameOut="$(uname -s)"
case "${unameOut}" in
    Linux*)     TMP=$(mktemp -p "$BASE" -d);;
    Darwin*)
        A=$(mktemp -d -t "drand")
        mv $A "/tmp/$(basename $A)"
        TMP="/tmp/$(basename $A)"
    ;;
esac
GROUPDIR="$TMP/groups"
GROUPFILE="$GROUPDIR/drand_group.toml"
CERTSDIR="$TMP/certs"
LOGSDIR="$TMP/logs"
IMG="dedis/drand:latest"
DRAND_PATH="src/github.com/dedis/drand"
DOCKERFILE="$GOPATH/$DRAND_PATH/Dockerfile"
NET="drand"
SUBNET="192.168.0."
PORT="80"
GOROOT=$(go env GOROOT)
# go run $GOROOT/src/crypto/tls/generate_cert.go --rsa-bits 1024 --host 127.0.0.1,::1,localhost --ca --start-date "Jan 1 00:00:00 1970" --duration=1000000h

function checkSuccess() {
    if [ "$1" -eq 0 ]; then
        return
    else
        echo "TEST <$2>: FAILURE"
        cleanup
        exit 1
    fi
}


function convert() {
    return printf -v int '%d\n' "$1" 2>/dev/null
}

if [ "$#" -gt 0 ]; then
    #n=$(convert "$1")
    if [ "$1" -gt 4 ]; then
        N=$1
    else
        echo "./run_local.sh <N> : N needs to be an integer > 4"
        exit 1
    fi
fi

## build the test travis image
function build() {
    echo "[+] Building the docker image $IMG"
    docker build -t "$IMG" .  > /dev/null
}

# associative array in bash 4
# https://stackoverflow.com/questions/1494178/how-to-define-hash-tables-in-bash
addresses=()
certs=()
tlskeys=()
certFile="/server.pem" ## certificate path on every container
keyFile="/key.pem" ## server private tls key path on every container

# run does the following:
# - creates the docker network
# - creates the individual keys under the temporary folder. Each node has its own
# folder named "nodeXX", where XX is the node's number.
# - create the group file
# - runs the whole set of nodes
# run takes one argument: foreground
# If foreground is true, then the last docker node runs in the foreground.
# If foreground is false, then all docker nodes run in the background.
function run() {
    echo "[+] Create the docker network $NET with subnet ${SUBNET}0/24"
    docker network create "$NET" --subnet "${SUBNET}0/24" > /dev/null 2> /dev/null

    echo "[+] Create the sub directories"
    mkdir -m 740 $CERTSDIR
    mkdir -m 740 $LOGSDIR
    mkdir -m 740 $GROUPDIR

    seq=$(seq 1 $N)
    rseq=$(seq $N -1 1)


    #sequence=$(seq $N -1 1)
    # creating the keys and compose part for each node
    echo "[+] Generating all private key pairs and certificates..."
    for i in $seq; do
        # gen key and append to group
        data="$TMP/node$i/"
        host="${SUBNET}2$i"
        addr="$host:$PORT"
        addresses+=($addr)
        mkdir -m 740 -p "$data"
        #drand keygen --keys "$data" "$addr" > /dev/null
        public="key/drand_id.public"
        volume="$data:/root/.drand/:z" ## :z means shareable with other containers
        allVolumes[$i]=$volume
        docker run --rm --volume ${allVolumes[$i]} $IMG generate-keypair "$addr" > /dev/null
            #allKeys[$i]=$data$public
        cp $data$public $TMP/node$i.public
        ## all keys from docker point of view
        allKeys[$i]=/tmp/node$i.public

        ## quicker generation with 1024 bits
        cd $data
        go run $GOROOT/src/crypto/tls/generate_cert.go --host $host --rsa-bits 1024
        certs+=("$(pwd)/cert.pem")
        tlskeys+=("$(pwd)/key.pem")
        cp cert.pem  $CERTSDIR/server-$i.cert
        echo "[+] Generated private/public pair + certificate for $addr"
    cd ..
    done

    ## generate group toml
    #echo $allKeys
    echo "[+] Generating group file"
    docker run --rm -v $TMP:/tmp:z $IMG "--folder" "$TMP" group "${allKeys[@]}"
    echo "[+] Group file generated at $TMP/groups/drand_group.toml"
    echo "[+] Starting all drand nodes sequentially..."
    for i in $rseq; do
        echo "[+] preparing for node $i"
        idx=`expr $i - 1`
        # gen key and append to group
        data="$TMP/node$i/"
        logFile="$LOGSDIR/node$i.log"
        groupFile="$data""drand_group.toml"
        cp $GROUPFILE $groupFile
        dockerGroupFile="/root/.drand/drand_group.toml"


        drandCmd=("start" "--tls-cert" "$certFile" "--tls-key" "$keyFile")
        args=(run --rm --name node$i --net $NET  --ip ${SUBNET}2$i) ## ip
        args+=("--volume" "${allVolumes[$i]}") ## config folder
        args+=("--volume" "$CERTSDIR:/certs:z") ## set of whole certs
        args+=("--volume" "${certs[$idx]}:$certFile") ## server cert
        args+=("--volume" "${tlskeys[$idx]}:$keyFile") ## server priv key
        args+=("-d") ## detached mode
        #echo "--> starting drand node $i: ${SUBNET}2$i"
        if [ "$i" -eq 1 ]; then
            drandCmd+=("--leader")
            if [ "$1" = true ]; then
                # running in foreground
                echo "[+] Running in foreground!"
                unset 'args[${#args[@]}-1]'
            fi
            echo "[+] Starting the leader of the dkg ($i)"
        else
            echo "[+] Starting node $i "
        fi
        drandCmd+=($dockerGroupFile)
        docker ${args[@]} "$IMG" "${drandCmd[@]}" > /dev/null
        docker logs -f node$i > $logFile &
        sleep 0.1
    done
}

function cleanup() {
    echo "[+] Cleaning up the docker containers..."
    docker stop $(docker ps -a -q) > /dev/null 2>/dev/null
    docker rm -f $(docker ps -a -q) > /dev/null 2>/dev/null
}

function fetchTest() {
    nindex=$1
    rootFolder="$TMP/node$nindex"
    distPublic="$rootFolder/groups/dist_key.public"
    serverId="/key/drand_id.public"
    drandVol="$rootFolder$serverId:$serverId"
    serverCert="$CERTSDIR/server-$nindex.cert"
    serverCertDocker="/server.cert"
    serverCertVol="$serverCert:$serverCertDocker"
    dockerGroupFile="/root/.drand/drand_group.toml"
    drandArgs=("get" "private")
    drandArgs+=("--tls-cert" "$serverCertDocker" "--nodes" "${addresses[$nindex]}" "$dockerGroupFile")
    echo "---------------------------------------------"
    echo "              Private Randomness             "
    docker run --rm --net $NET --ip "${SUBNET}10" -v "$drandVol" -v "$serverCertVol" $IMG "${drandArgs[@]}"
    echo "---------------------------------------------"
    checkSuccess $? "verify randomness encryption"
    echo "---------------------------------------------"
    echo "               Public Randomness             "
    drandPublic="/dist_public.toml"
    drandVol="$distPublic:$drandPublic"
    drandArgs=( "get" "public")
    drandArgs+=("--tls-cert" "$serverCertDocker")
    idx=`expr $nindex - 1`
    drandArgs+=("--nodes" "${addresses[$idx]}" "$dockerGroupFile")
    docker run --rm --net $NET --ip "${SUBNET}11" -v "$drandVol" -v "$serverCertVol" $IMG "${drandArgs[@]}"
    checkSuccess $? "verify signature?"
    echo "---------------------------------------------"
}

cleanup

## END OF LIBRARY
if [ "${#BASH_SOURCE[@]}" -gt "1" ]; then
    echo "[+] run_local.sh used as library -> not running"
    return 0;
fi

## RUN LOCALLY SCRIPT
trap cleanup SIGINT
build
run false
echo "[+] Waiting 3s to get some beacons..."
sleep 3
while true;
nindex=1
do
    fetchTest $nindex true
    sleep 2
done
