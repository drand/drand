#!/bin/bash

#set -x

# This script contains two parts.
# The first part is meant as a library, declaring the variables and functions to spins off drand containers
# The second part is triggered when this script is actually ran, and not
# sourced. This part calls the function to setup the drand containers and run
# them. It produces produce randomness in a temporary folder..
#
N=6 ## final number of nodes in total - only N-1 will be running
OLDN=5 ## starting number of nodes
thr=4
period="4s"
BASE="/tmp/drand-docker"
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
echo "[+] Setting up tests in $TMP folder"
GROUPFILE="$TMP/group.toml"
CERTSDIR="$TMP/certs"
LOGSDIR="$TMP/logs"

## file in test/test-integration
curr=$(pwd)
cd ../../
root=$(pwd)
cd "$curr"
IMG="dedis/drand:latest"
DOCKERFILE="$root/Dockerfile"
NET="drand"
SUBNET="192.168.215."
PORT="80"
GOROOT=$(go env GOROOT)
GENESISOFFSET=20
GENESISTIME=""
TRANSITIONOFFSET=40
TRANSITIONTIME=""
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
    echo " ---- GOPATH $GOPATH"
    docker build -t "$IMG" -f "$DOCKERFILE" "$root"
    img="byrnedo/alpine-curl"
    ## XXX make curl work without the "-k" option
    docker pull $img > /dev/null

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
    newRseq=$(seq $N -1 2)


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
        docker run --rm --volume ${allVolumes[$i]} $IMG generate-keypair "$addr" # > /dev/null
        cp $data$public $TMP/node$i.public
        ## all keys from docker point of view
        allKeys[$i]=/tmp/node$i.public

        ## quicker generation with 1024 bits
        cd $data
        go run $GOROOT/src/crypto/tls/generate_cert.go --host $host --rsa-bits 1024 # > /dev/null 2>& 1
        certs+=("$(pwd)/cert.pem")
        tlskeys+=("$(pwd)/key.pem")
        cp cert.pem  $CERTSDIR/server-$i.cert
        echo "[+] Done generating key pair and certificate for drand node $addr"
    done

    ## generate group toml from the first 5 nodes ONLY
    ## We're gonna add the last one later on
    GENESISTIME=$(($(date +%s) + $GENESISOFFSET ))
    docker run --rm -v $TMP:/tmp:z $IMG group --out /tmp/group.toml --period "$period" --genesis $GENESISTIME "${allKeys[@]:0:$OLDN}"  #> /dev/null
    echo "[+] Group file generated at $GROUPFILE for genesis time $GENESISTIME"
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
        drandCmd=("start" "--verbose" "--certs-dir" "/certs")
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
                echo "[+] Running in foreground!"
                unset 'args[${#args[@]}-1]'
            fi
            echo "[+] Starting (DKG coordinator) node $i"
        else
            echo "[+] Starting node $i "
        fi
        docker ${args[@]} "$IMG" "${drandCmd[@]}" #> /dev/null
        docker logs -f node$i > $logFile &
        #docker logs -f node$i &

        sleep 0.5
       
        # check if the node is up 
        pingNode $name

        if [ "$i" -eq 1 ]; then
            docker exec -it $name drand share --leader "$dockerGroupFile" > /dev/null
            # check the group
            docker exec -it $name drand check-group "$dockerGroupFile" \
            --certs-dir /certs > /dev/null
            checkSuccess $? "[-] Group checking has failed. Stopping now."
        else
            docker exec -d $name drand share "$dockerGroupFile" > /dev/null
        fi

        if [ "$i" -eq 1 ]; then
            while true; do
                docker exec -it $name drand get cokey --verbose \
                    --tls-cert "$certFile" "$dockerGroupFile" > /dev/null
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

    ## make them do at least one round
    echo "[+] Sleeping to wait for genesis start time + one period round"
    sleep $(($GENESISTIME - $(date +%s)))
    echo "   - Genesis time reached: $(date +%s)"
    sleep $period
    echo "   - Second round time reached: $(date +%s)"

    ## we look at the second node since the first node will be out during the
    ## resharing
    share1Path="$TMP/node2/groups/dist_key.private"
    share1Hash=$(eval "$SHA $share1Path")
    group1Path="$TMP/node2/groups/drand_group.toml"
    group1Hash=$(eval "$SHA $group1Path")
    ## replace the old reference of the group to the new 
    ## reference with  the distributed key included.
    ## we take the one generated by node2, but they're all 
    ## the same
    cp "$group1Path" "$GROUPFILE.1"

    # trying to add the last node to the group
   ## TRANSITIONTIME=$(($(date +%s) + $TRANSITIONOFFSET ))
   ## echo "[+] Generating new group with additional node  - transition at $TRANSITIONTIME"
   ## dockerPath=/tmp/group.toml
   ## docker run --rm -v $TMP:/tmp:z $IMG group --out $dockerPath --from $dockerPath --transition $TRANSITIONTIME --threshold $thr "${allKeys[@]}" #> /dev/null
   ## cp $GROUPFILE "$GROUPFILE.2"

   ## i=6
   ## echo "[+] Starting node additional node $i"
   ## idx=`expr $i - 1`
   ## # gen key and append to group
   ## data="$TMP/node$i/"
   ## logFile="$LOGSDIR/node$i.log"
   ## groupFile="$data""drand_group.toml"
   ## cp $GROUPFILE $groupFile
   ## groupFileOld="$data""drand_group.toml.old"
   ## cp $group1Path $groupFileOld
   ## dockerGroupFile="/root/.drand/drand_group.toml"

   ## name="node$i"
   ## drandCmd=("start" "--verbose" "--certs-dir" "/certs")
   ## drandCmd+=("--tls-cert" "$certFile" "--tls-key" "$keyFile")
   ## args=(run --rm --name $name --net $NET  --ip ${SUBNET}2$i) ## ip
   ## args+=("--volume" "${allVolumes[$i]}") ## config folder
   ## args+=("--volume" "$CERTSDIR:/certs:z") ## set of whole certs
   ## args+=("--volume" "${certs[$idx]}:$certFile") ## server cert
   ## args+=("--volume" "${tlskeys[$idx]}:$keyFile") ## server priv key
   ## args+=("-d") ## detached mode
   ## docker ${args[@]} "$IMG" "${drandCmd[@]}" # > /dev/null
   ## docker logs -f node$i > $logFile &
   ## #docker logs -f node$i  &
   ## # check if the node is up 
   ## pingNode $name 
   ## timeout="15s"

   ## ## stop the first node
   ## echo "[+] Stopping the first node"
   ## docker stop "node1" # > /dev/null

   ## ## start all nodes BUT the first one -> try a resharing threshold.
   ## for i in $newRseq; do
   ##     idx=`expr $i - 1`
   ##     name="node$i"
   ##     data="$TMP/node$i/"
   ##     logFile="$LOGSDIR/node$i.log"
   ##     nodeGroupFile="$data""drand_group.toml"
   ##     cp "$GROUPFILE.1" "$nodeGroupFile.1"
   ##     cp "$GROUPFILE.2" "$nodeGroupFile.2"
   ##     newGroup="/root/.drand/drand_group.toml.2"
   ##     oldGroup="/root/.drand/drand_group.toml.1"
   ##     #oldGroup=$groupFileOld

   ##     name="node$i"
   ##      if [ "$i" -eq 2 ]; then
   ##         echo "[+] Start resharing command to leader $name"
   ##         docker exec -it $name drand share --leader --timeout "$timeout" "$newGroup" > /dev/null
   ##     elif [ "$i" -eq "$N" ]; then
   ##         echo "[+] Issuing resharing command to NEW node $name"
   ##         docker exec -d $name drand share --from "$oldGroup" --timeout "$timeout" "$newGroup" > /dev/null
   ##     else
   ##         echo "[+] Issuing resharing command to node $name"
   ##         docker exec -d $name drand share --timeout "$timeout" "$newGroup" > /dev/null
   ##     fi
   ## done

   ## ## check if the two groups file are different
   ## group2Hash=$(eval "$SHA $group1Path")
   ## if [ "$group1Hash" = "$group2Hash" ]; then
   ##     echo "[-] Checking group file... Same as before - WRONG."
   ##     exit 1
   ## else
   ##     echo "[+] Checking group file... New one created !"
   ## fi

   ## share2Hash=$(eval "$SHA $share1Path")
   ## if [ "$share1Hash" = "$share2Hash" ]; then
   ##     echo "[-] Checking private shares... Same as before - WRONG"
   ##     exit 1
   ## else
   ##     echo "[+] Checking private shares... New ones !"
   ## fi

   ## toSleep=$(($TRANSITIONTIME - $(date +%s)))
   ## echo "[+] Sleeping $toSleep + $period seconds to wait transition time + one round"
   ## sleep $toSleep
   ## echo "   - Transition time reached: $(date +%s)"
   ## sleep $period
   ## echo "   - Second round time reached after transition $(date +%s)"
}

function pingNode() {
    while true; do
        docker exec -it $1 drand ping > /dev/null
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
    arrIndex=$(expr $nindex - 1)
    echo "fetchTest() $nindex $arrIndex ${addresses[0]} ${addresses[$arrIndex]}"
    rootFolder="$TMP/node$nindex"
    #distPublic="$rootFolder/groups/dist_key.public"
    # trying to wait until dist_key.public is there
    distPublic="$TMP/node1/groups/dist_key.public"

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
    checkSuccess $? "verify randomness encryption"
    echo "---------------------------------------------"

    echo "---------------------------------------------"
    echo "               Public Randomness             "
    drandArgs=("get" "public")
    drandArgs+=("--tls-cert" "$serverCertDocker")
    drandArgs+=("--nodes" "${addresses[$arrIndex]}") 
    drandArgs+=("$dockerGroupToml")
    docker run --rm --net $NET --ip "${SUBNET}11" -v "$serverCertVol" \
                                                  -v "$groupVolume" \
                                                  $IMG "${drandArgs[@]}"
    checkSuccess $? "verify signature?"
    echo "---------------------------------------------"

    echo "---------------------------------------------"
    echo "[+] Public Randomness with CURL"
    img="byrnedo/alpine-curl"
    ## XXX make curl work without the "-k" option
    docker run --rm --net $NET --ip "${SUBNET}12" -v "$serverCertVol" \
                $img -s -k --cacert "$serverCertDocker" \
                -H "Content-type: application/json" \
                "https://${addresses[$arrIndex]}/api/public" | python -m json.tool

    checkSuccess $? "verify REST API for public randomness"
    echo "---------------------------------------------"

    echo "---------------------------------------------"
    echo "[+] Distributed key with CURL"
    docker run --rm --net $NET --ip "${SUBNET}12" -v "$serverCertVol" \
                $img -s -k --cacert "$serverCertDocker" \
                -H "Content-type: application/json" \
                "https://${addresses[$arrIndex]}/api/info/distkey" | python -m json.tool
    checkSuccess $? "verify REST API for getting distributed key"
    echo "---------------------------------------------"
    echo "[+] Group information with CURL"
    docker run --rm --net $NET --ip "${SUBNET}12" -v "$serverCertVol" \
                $img -s -k --cacert "$serverCertDocker" \
                -H "Content-type: application/json" \
                "https://${addresses[$arrIndex]}/api/info/group" | python -m json.tool
    checkSuccess $? "verify REST API for getting group info"
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
sleep "$period"
while true;
nindex=2
do
    fetchTest $nindex true
    sleep 2
done
