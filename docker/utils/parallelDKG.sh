#bin/sh

## Test beacon_1
## Generate keys
nohup docker exec -u drand drand_0 /bin/sh -c 'drand generate-keypair --tls-disable --id beacon_1 "drand_0:8080"' &
sleep 1
nohup docker exec -u drand drand_1 /bin/sh -c 'drand generate-keypair --tls-disable --id beacon_1 "drand_1:8180"' &
sleep 1
nohup docker exec -u drand drand_2 /bin/sh -c 'drand generate-keypair --tls-disable --id beacon_1 "drand_2:8280"' &
sleep 1
nohup docker exec -u drand drand_3 /bin/sh -c 'drand generate-keypair --tls-disable --id beacon_1 "drand_3:8380"' &
sleep 1

# Start leader for default beacon
nohup docker exec -u drand drand_3 /bin/sh -c 'drand share --leader --nodes 4 --threshold 3 --period "5s"' &
sleep 5

# Start leader for beacon_1
nohup docker exec -u drand drand_3 /bin/sh -c 'drand share --leader --nodes 4 --threshold 3 --period "60s" --id beacon_1' &
sleep 5

# Start the rest of the nodes
# default
nohup docker exec -u drand drand_2 /bin/sh -c 'drand share --connect drand_3:8380 --tls-disable' &
sleep 1
nohup docker exec -u drand drand_1 /bin/sh -c 'drand share --connect drand_3:8380 --tls-disable' &
sleep 1
nohup docker exec -u drand drand_0 /bin/sh -c 'drand share --connect drand_3:8380 --tls-disable' &
sleep 1
# beacon_1
nohup docker exec -u drand drand_2 /bin/sh -c 'drand share --connect drand_3:8380 --tls-disable --id beacon_1' &
sleep 1
nohup docker exec -u drand drand_1 /bin/sh -c 'drand share --connect drand_3:8380 --tls-disable --id beacon_1' &
sleep 1
nohup docker exec -u drand drand_0 /bin/sh -c 'drand share --connect drand_3:8380 --tls-disable --id beacon_1' &
