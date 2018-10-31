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

N=6 ## final number of nodes
OLDN=5 ## starting number of nodes
BASE="/tmp/drand"
SHA="sha256sum"
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
        SHA="shasum -a 256"
    ;;
esac
GROUPFILE="$TMP/group.toml"
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
    echo "[+] Building docker image $IMG"
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
    echo "[+] Creating docker network $NET with subnet ${SUBNET}0/24"
    docker network create "$NET" --subnet "${SUBNET}0/24" > /dev/null 2> /dev/null

    echo "[+] Creating the certificate directory"
    mkdir -m 740 $CERTSDIR
    mkdir -m 740 $LOGSDIR

    seq=$(seq 1 $N)
    oldRseq=$(seq $OLDN -1 1)
    newRseq=$(seq $N -1 1)


    #sequence=$(seq $N -1 1)
    # creating the keys and compose part for each node
    echo "[+] Generating key pairs and certificates for drand nodes"
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
        echo "[+] Done generating key pair and certificate for drand node $addr"
    done

    ## generate group toml from the first 5 nodes ONLY
    ## We're gonna add the last one later on
    period="2s"
    docker run --rm -v $TMP:/tmp:z $IMG group --out /tmp/group.toml --period "$period" "${allKeys[@]:0:$OLDN}"  #> /dev/null
    echo "[+] Group file generated at $GROUPFILE"
    cp $GROUPFILE "$GROUPFILE.1"
    echo "[+] Starting all drand nodes sequentially..."
    for i in $oldRseq; do
        idx=`expr $i - 1`
        # gen key and append to group
        data="$TMP/node$i/"
        logFile="$LOGSDIR/node$i.log"
        groupFile="$data""drand_group.toml"
        cp $GROUPFILE $groupFile
        dockerGroupFile="/root/.drand/drand_group.toml"

        name="node$i"
        drandCmd=("--verbose" "2" "start" "--certs-dir" "/certs")
        drandCmd+=("--tls-cert" "$certFile" "--tls-key" "$keyFile")
        args=(run --rm --name $name --net $NET  --ip ${SUBNET}2$i) ## ip
        args+=("--volume" "${allVolumes[$i]}") ## config folder
        args+=("--volume" "$CERTSDIR:/certs:z") ## set of whole certs
        args+=("--volume" "${certs[$idx]}:$certFile") ## server cert
        args+=("--volume" "${tlskeys[$idx]}:$keyFile") ## server priv key
        args+=("-d") ## detached mode
        #echo "--> starting drand node $i: ${SUBNET}2$i"
        if [ "$i" -eq 1 ]; then
            if [ "$1" = true ]; then
                # running in foreground
                # XXX should be finished
                echo "[+] Running in foreground!"
                unset 'args[${#args[@]}-1]'
            fi
            echo "[+] Starting (DKG coordinator) node $i"
        else
            echo "[+] Starting node $i "
        fi
        docker ${args[@]} "$IMG" "${drandCmd[@]}" > /dev/null
        docker logs -f node$i > $logFile &
        #docker logs -f node$i &

        sleep 0.5
       
        # check if the node is up 
        pingNode $name

        if [ "$i" -eq 1 ]; then
            docker exec -it $name drand share --leader "$dockerGroupFile" > /dev/null
        else
            docker exec -d $name drand share "$dockerGroupFile"
        fi

        if [ "$i" -eq 1 ]; then
            while true; do
                docker exec -it $name drand --verbose 2 get cokey \
                    --tls-cert "$certFile" "$dockerGroupFile"
                if [ "$?" -eq 0 ]; then
                    echo "[+] Successfully retrieve distributed key from leader"
                    break
                fi
                echo "[-] Can't get distributed key from root node. Waiting..."
                sleep 1
            done
        fi

    done

    # trying to wait until dist_key.public is there
    dpublic="$TMP/node1/groups/dist_key.public"
    while true; do
        if [ -f "$dpublic" ]; then
            echo "[+] Distributed public key file found."
            break;
        fi
        echo "[-] Distributed public key file NOT found. Waiting a bit more..."
        sleep 1
    done

    share1Path="$TMP/node1/groups/dist_key.private"
    share1Hash=$(eval "$SHA $share1Path")
    group1Path="$TMP/node1/groups/drand_group.toml"
    group1Hash=$(eval "$SHA $group1Path")

    # trying to add the last node to the group
    echo "[+] Generating new group with additional node"
    docker run --rm -v $TMP:/tmp:z $IMG group --out /tmp/group.toml --period "$period" "${allKeys[@]}" > /dev/null
    cp $GROUPFILE "$GROUPFILE.2"

    i=6
    echo "[+] Starting node additional node $i"
    idx=`expr $i - 1`
    # gen key and append to group
    data="$TMP/node$i/"
    logFile="$LOGSDIR/node$i.log"
    groupFile="$data""drand_group.toml"
    cp $GROUPFILE $groupFile
    dockerGroupFile="/root/.drand/drand_group.toml"

    name="node$i"
    drandCmd=("--verbose" "2" "start" "--certs-dir" "/certs")
    drandCmd+=("--tls-cert" "$certFile" "--tls-key" "$keyFile")
    args=(run --rm --name $name --net $NET  --ip ${SUBNET}2$i) ## ip
    args+=("--volume" "${allVolumes[$i]}") ## config folder
    args+=("--volume" "$CERTSDIR:/certs:z") ## set of whole certs
    args+=("--volume" "${certs[$idx]}:$certFile") ## server cert
    args+=("--volume" "${tlskeys[$idx]}:$keyFile") ## server priv key
    args+=("-d") ## detached mode
    docker ${args[@]} "$IMG" "${drandCmd[@]}" > /dev/null
    docker logs -f node$i > $logFile &
    #docker logs -f node$i  &
    # check if the node is up 
    pingNode $name 

    for i in $newRseq; do
        idx=`expr $i - 1`
        name="node$i"
        data="$TMP/node$i/"
        logFile="$LOGSDIR/node$i.log"
        nodeGroupFile="$data""drand_group.toml"
        cp "$GROUPFILE.1" "$nodeGroupFile.1"
        cp "$GROUPFILE.2" "$nodeGroupFile.2"
        newGroup="/root/.drand/drand_group.toml.2"
        oldGroup="/root/.drand/drand_group.toml.1"

        name="node$i"
         if [ "$i" -eq 1 ]; then
            echo "[+] Start resharing command to leader $name"
            docker exec -it $name drand share --leader "$newGroup" > /dev/null
        elif [ "$i" -eq "$N" ]; then
            echo "[+] Issuing resharing command to NEW node $name"
            docker exec -d $name drand share --from "$oldGroup" "$newGroup"
        else
            echo "[+] Issuing resharing command to node $name"
            docker exec -d $name drand share "$newGroup"
        fi
    done

    ## check if the two groups file are different
    group2Hash=$(eval "$SHA $group1Path")
    if [ "$group1Hash" = "$group2Hash" ]; then
        echo "[-] Checking group file... Same as before - WRONG."
        exit 1
    else
        echo "[+] Checking group file... New one created !"
    fi

    share2Hash=$(eval "$SHA $share1Path")
    if [ "$share1Hash" = "$share2Hash" ]; then
        echo "[-] Checking private shares... Same as before - WRONG"
        exit 1
    else
        echo "[+] Checking private shares... New ones !"
    fi

}

function pingNode() {
    while true; do
        docker exec -it $1 drand --verbose 2 ping #> /dev/null
        if [ $? == 0 ]; then
            #echo "$name is UP and RUNNING"
            break
        fi
        sleep 0.2
    done


}

function cleanup() {
    echo "[+] Cleaning up docker containers"
    docker stop $(docker ps -a -q) > /dev/null 2>/dev/null
    docker rm -f $(docker ps -a -q) > /dev/null 2>/dev/null
}

function fetchTest() {
    nindex=$1
    rootFolder="$TMP/node$nindex"
    distPublic="$rootFolder/groups/dist_key.public"
    serverCert="$CERTSDIR/server-$nindex.cert"
    serverCertDocker="/server.cert"
    serverCertVol="$serverCert:$serverCertDocker"
    groupToml="$rootFolder/groups/drand_group.toml"
    dockerGroupToml="/group.toml"
    groupVolume="$groupToml:$dockerGroupToml"
    drandArgs=("get" "private")
    drandArgs+=("--tls-cert" "$serverCertDocker" "$dockerGroupToml")
    echo "[+] Series of tests using drand cli tool"
    echo "---------------------------------------------"
    echo "              Private Randomness             "
    docker run --rm --net $NET --ip "${SUBNET}10" -v "$serverCertVol" \
                                                  -v "$groupVolume" \
                                                  $IMG "${drandArgs[@]}"
    echo "---------------------------------------------"
    checkSuccess $? "verify randomness encryption"

    echo "---------------------------------------------"
    drandArgs=("--verbose" "2" "get" "cokey")
    drandArgs+=("--tls-cert" "$serverCertDocker" "$dockerGroupToml")
    docker run --rm --net $NET --ip "${SUBNET}10" -v "$serverCertVol" \
                                                  -v "$groupVolume" \
                                                  $IMG "${drandArgs[@]}"
    checkSuccess $? "verify signature?"
    echo "---------------------------------------------"

    echo "---------------------------------------------"
    echo "               Public Randomness             "
    drandArgs=("get" "public")
    drandArgs+=("--tls-cert" "$serverCertDocker" "$dockerGroupToml")
    docker run --rm --net $NET --ip "${SUBNET}11" -v "$serverCertVol" \
                                                  -v "$groupVolume" \
                                                  $IMG "${drandArgs[@]}"
    checkSuccess $? "verify signature?"
    echo "---------------------------------------------"
    echo "[+] Public Randomness with CURL"
    img="byrnedo/alpine-curl"
    ## XXX make curl work without the "-k" option
    docker pull $img
    docker run --rm --net $NET --ip "${SUBNET}12" -v "$serverCertVol" \
                $img -s -k --cacert "$serverCertDocker" \
                "https://${addresses[0]}/public" | python -m json.tool

    checkSuccess $? "verify REST API for public randomness"
    echo "[+] Distributed key with CURL"
    docker run --rm --net $NET --ip "${SUBNET}12" -v "$serverCertVol" \
                $img -s -k --cacert "$serverCertDocker" \
                "https://${addresses[0]}/info/dist_key" | python -m json.tool
    checkSuccess $? "verify REST API for getting distributed key"
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
echo "[+] Waiting to get some beacons"
sleep 3
while true;
nindex=1
do
    fetchTest $nindex true
    sleep 2
done
