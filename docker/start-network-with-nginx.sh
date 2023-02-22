#!/bin/sh

###
#   This script sets up a network with 3 nodes, runs an initial distributed key generation, then spins up another node behind nginx
#   and runs a resharing process to include that node in the network
###

# first lets kill any existing runs
docker-compose --file docker-compose-nginx.yml down
./cleanup.sh

./start-network.sh

# then let's create a volume for the nginx drand node and put a keypair on it pointing to the grpc port
docker volume create drand-nginx
docker run --volume drand-nginx:/data/drand drandorg/go-drand:v1.5.3-testnet generate-keypair  --folder /data/drand/.drand --tls-disable --id default nginx:81
docker-compose --file docker-compose-nginx.yml up --detach

# start the resharing as leader
echo [+] starting the resharing as leader
docker exec --detach -e DRAND_SHARE_SECRET=deadbeefdeadbeefdeadbeefdeadbeef drand1 sh -c "drand share --leader   --control 8888 --tls-disable --nodes 4 --threshold 3 --id default --transition"

# sleep a little so the leader is set up
sleep 1

echo [+] existing nodes are joining the resharing
# run the resharing for the two existing nodes
docker exec --detach -e DRAND_SHARE_SECRET=deadbeefdeadbeefdeadbeefdeadbeef drand2 sh -c "drand share --connect drand1:8010 --tls-disable --id default --transition"
docker exec --detach -e DRAND_SHARE_SECRET=deadbeefdeadbeefdeadbeefdeadbeef drand3 sh -c "drand share --connect drand1:8010 --tls-disable --id default --transition"


# now we join the resharing from the nginx container
echo [+] the nginx node is joining the resharing

## first we steal the existing group file from another node
group_contents=$(docker exec drand1 sh -c cat /data/drand/.drand/multibeacon/default/groups/drand_group.toml)
docker cp drand1:/data/drand/.drand/multibeacon/default/groups/drand_group.toml ./group.toml
docker cp ./group.toml drand-nginx:/data/drand/group.toml
rm -rf ./group.toml

## then we run the resharing command
docker exec -e DRAND_SHARE_SECRET=deadbeefdeadbeefdeadbeefdeadbeef --detach drand-nginx sh -c "drand share --connect drand1:8010 --tls-disable  --from /data/drand/group.toml"

# let's wait until the node reports healthy (i.e. it has caught up with the network)
echo [+] Waiting for the resharing to finish - could take up to a minute!
attempts=60

while :
do
  ### if it isn't working after a bunch of attempts, it probably failed
  if [ "$attempts" -eq 0 ]; then
    echo [-] the nginx node didn\'t finish the resharing successfully - check the container logs with '`docker logs -f drand-nginx`'
    exit 1
  fi

  ### once the node reports healthy, we know it has caught up and is part of the network
  response=$(curl --silent --head localhost:80/health)
  if [[ $? -eq 0 && $response =~ "200 OK" ]]; then
    break
  fi

  attempts=$(($attempts - 1))
  sleep 1
done

echo [+] resharing completed successfully
