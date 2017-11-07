#!/bin/bash 
#set -x 
# This script contains two parts.
# The first part is meant as a library, declaring the variables and functions to spins off drand containers 
# The second part is triggered when this script is actually ran, and not
# sourced. This part calls the function to setup the drand containers and run
# them. It produces produce randomness in a temporary folder..
#
# NOTE: Using docker compose should give a higher degree of flexibility and
# composability. However I had trouble with spawning the containers too fast and
# having weird binding errors: port already in use. I rolledback to simple
# docker scripting. One of these, one should try to do it in docker-compose.
## number of nodes

N=6
TMP="/tmp/drand"
if [ -d "$TMP" ]; then
    echo "[+] /tmp/drand already exists. Need sudo to remove it because drand
    runs on root inside the container:"
    sudo rm -rf $TMP
    mkdir $TMP
fi
GROUPFILE="$TMP/group.toml"
IMG="dedis/drand"
DRAND_PATH="src/github.com/dedis/drand"
DOCKERFILE="$GOPATH/$DRAND_PATH/Dockerfile"
NET="drand"
SUBNET="192.168.0."
PORT="800"

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
    docker build -t "$IMG" -f "$DOCKERFILE" .
}

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

    sequence=$(seq $N -1 1)
    #sequence=$(seq $N -1 1)
    # creating the keys and compose part for each node
    echo "[+] Generating the private keys..." 
    for i in $sequence; do
        # gen key and append to group
        data="$TMP/node$i/"
        addr="${SUBNET}2$i:$PORT$i"
        mkdir -p "$data"
        #drand keygen --keys "$data" "$addr" > /dev/null 
        public="drand_id.public"
        volume="$data:/root/.drand/"
        allVolumes[$i]=$volume
        docker run --rm --volume ${allVolumes[$i]} $IMG keygen "$addr" > /dev/null
            #allKeys[$i]=$data$public
        cp $data$public $TMP/node$i.public
        allKeys[$i]=/tmp/node$i.public
    done

    ## generate group toml
    #echo $allKeys
    docker run --rm -v $TMP:/tmp $IMG group --group /tmp/group.toml "${allKeys[@]}" > /dev/null
    echo "[+] Group file generated at $GROUPFILE"
    echo "[+] TO LIST THE BEACONS:"
    echo
    echo "  ls $TMP/node1/beacons/"
    echo
    echo "[+] TO VERIFY THE FIRST BEACON:"
    echo 
    echo "  docker run --rm -v $TMP:/tmp $IMG verify --distkey /tmp/node1/dist_key.public /tmp/node1/beacons/\$(ls $TMP/node1/beacons/ | head -n1)"
    echo 
    echo "[+] Starting all drand nodes sequentially..." 
    for i in $sequence; do
        # gen key and append to group
        data="$TMP/node$i/"
        cp $GROUPFILE "$data"drand_group.toml
        drandCmd=("run")
        detached="-d"
        args=(run --rm --name node$i --net $NET  --ip ${SUBNET}2$i --volume ${allVolumes[$i]} -d)
        #echo "--> starting drand node $i: ${SUBNET}2$i"
        if [ "$i" -eq 1 ]; then
            drandCmd+=("--leader" "--period" "2s")
            if [ "$1" = true ]; then
                # running in foreground
                unset 'args[${#args[@]}-1]'
            fi
            echo "[+] Starting the leader"
            docker ${args[@]} "$IMG" "${drandCmd[@]}"
        else
            docker ${args[@]} "$IMG" "${drandCmd[@]}" > /dev/null
        fi
        #drandCmd+=(>
        #echo "[+] Starting node$i at ${SUBNET}2$i with ${allVolumes[$i]}..."
        #docker run --rm --name node$i --net $NET \
                    #--ip ${SUBNET}2$i \
                    #--volume ${allVolumes[$i]} $detached "$IMG" "$cmd"

        sleep 0.1
        detached="-d"
    done
}

function cleanup() {
    echo "[+] Cleaning up the docker containers..." 
    docker rm -f $(docker ps -a -q)
}
## END OF LIBRARY 
if [ "${#BASH_SOURCE[@]}" -gt "1" ]; then
    return 0;
fi

## RUN LOCALLY SCRIPT
trap cleanup SIGINT
#build
run true
