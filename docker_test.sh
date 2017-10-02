#!/bin/bash 

#set -x

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

#umask 0000

# install latest binary, & generate dockerfile dynamically to get the right path
#echo "Generating dockerfile"
#cd $GOPATH/$DRAND_PATH
#echo $GOPATH/$DRAND_PATH
#go get ./...
#go install
#rm -f $DOCKERFILE
#docker rm -f $(docker ps -a -q) 2> /dev/null 
#cat >> $DOCKERFILE << EOF
#FROM dedis/drand:bn

#RUN mkdir -p /go/src/github.com/dedis/drand
#COPY . "/go/src/github.com/dedis/drand"
#WORKDIR "/go/src/github.com/dedis/drand"
#RUN go install

#ENTRYPOINT ["drand"]
#EOF

## build the test travis image
#echo "Building the $IMG image"
docker build -t "$IMG" -f "$DOCKERFILE" .

echo "Create network $NET with subnet ${SUBNET}0/24"
docker network create "$NET" --subnet "${SUBNET}0/24"

sequence=$(seq $N -1 1)
echo "Creating and running $N nodes"
# creating the keys and compose part for each node
for i in $sequence; do
    # gen key and append to group
    data="$TMP/node$i/"
    addr="${SUBNET}2$i:$PORT$i"
    mkdir -p "$data"
    echo "generating keys for $addr"
    drand keygen --keys "$data" "$addr" > /dev/null 
    public="drand_id.public"
    volume="$data:/root/.drand/"
    allKeys[$i]=$data$public
    allVolumes[$i]=$volume
done

## generate group toml
echo $allKeys
drand group --group "$GROUPFILE" ${allKeys[@]}
echo "GROUP FILE:"
cat $GROUPFILE

for i in $sequence; do
    # gen key and append to group
    data="$TMP/node$i/"
    cp $GROUPFILE "$data"drand_group.toml
    cmd="run"
    if [ "$i" -eq 1 ]; then
        cmd="$cmd --leader --period 2s"
    fi

    echo "Running docker container node$i at ${SUBNET}2$i with ${allVolumes[$i]}..."
    docker run --rm --name node$i --net $NET \
                --ip ${SUBNET}2$i \
                --volume ${allVolumes[$i]} -d $IMG $cmd
    sleep 0.1
done

function cleanup() {
    echo "removing containers ..." 
    docker rm -f $(docker ps -a -q)
}

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
docker logs node1
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
drand verify --distkey "$distPublic" "$rootFolder/beacons/$sigFile"
checkSuccess $? "verify signature?"

echo "TESTS OK"
cleanup
