#!/bin/sh


num_of_nodes=3
docker_image_version=v1.5.2-testnet

### first we check if docker and docker-compose are installed

has_docker=$(docker version | echo "$?")

if [ ! has_docker ]; then
  echo [-] You must have docker installed to run the network
  exit 1
fi

has_docker_compose=$(docker-compose --version | echo "$?")

if [ ! has_docker_compose ]; then
  echo [-] You must have docker-compose installed to run the network
  exit 1
fi

echo [+] Pulling the official drand docker image $docker_image_version
docker pull drandorg/go-drand:$docker_image_version 1>/dev/null 


### then we create a volume for each of those nodes
echo [+] Creating docker volumes for $num_of_nodes nodes

for i in $(seq 1 $num_of_nodes);
do
  docker volume create drand$i 1>/dev/null
done

### next we're going to generate a keypair on each of those volumes
echo [+] Generating a default network keypair for each node

for i in $(seq 1 $num_of_nodes);
do
  # these will end up on drand1:8010, drand2:8020, drand3:8030, etc
  # note they map to the container's mapped ports, but the internal ports; internally the services still listen on 8080
  path=drand$i:80${i}0
  docker run --rm --volume drand$i:/data/drand drandorg/go-drand:$docker_image_version generate-keypair  --folder /data/drand/.drand --tls-disable --id default $path 1>/dev/null
done

### now we start them all using docker-compose as it'll be easy to spin up and down
echo [+] Starting all the nodes using docker-compose

docker-compose -f docker-compose-network.yml up -d


### sleep to let the nodes start up
sleep 5

### now we run the initial distributed key generation
### we're going to use the first node as the leader node

echo [+] Starting distributed key generation for the leader

# we start the DKG and send it to the background; 
docker exec --env DRAND_SHARE_SECRET=deadbeefdeadbeefdeadbeefdeadbeef --detach drand1 sh -c "drand share --id default --leader --nodes 3 --threshold 2 --period 15s --tls-disable"

# and sleep a second so the other nodes don't try and join before the leader has set up all its bits and bobs!
sleep 1
echo [+] Started distributed key generation for the leader

echo [+] Joining distributed key generation for the followers
for i in $(seq 2 $num_of_nodes);
do
  # we start the DKG and send it to the background
  docker exec --env DRAND_SHARE_SECRET=deadbeefdeadbeefdeadbeefdeadbeef --detach drand$i sh -c "drand share --id default --connect drand1:8010 --tls-disable"
done


### now we wait for the distributed key generation to be completed and the first round to be created

echo [+] Waiting for the DKG to finish - could take up to a minute!
attempts=60

while :
do
  ### if it isn't working after a bunch of attempts, it probably failed
  if [ "$attempts" -eq 0 ]; then
    echo [-] the DKG didn\'t finish successfully - check the container logs with '`docker logs -f drand1`'
    exit 1
  fi

  ### once the first round has been created, we know that the DKG happened succesfully
  response=$(curl --silent localhost:9010/public/1)
  if [[ $? -eq 0 && $response =~ "round" ]]; then
    break
  fi

  attempts=$(($attempts - 1))
  sleep 1
done

echo [+] Network running successfully!
