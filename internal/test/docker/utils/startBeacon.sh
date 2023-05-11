#bin/sh

## Default beacon
# Start leader
nohup docker exec -u drand drand_3 /bin/sh -c 'drand share --leader --nodes 4 --threshold 3 --period "5s"' &

sleep 3

# Start the rest of the nodes
nohup docker exec -u drand drand_2 /bin/sh -c 'drand share --connect drand_3:8380 --tls-disable' &
nohup docker exec -u drand drand_1 /bin/sh -c 'drand share --connect drand_3:8380 --tls-disable' &
nohup docker exec -u drand drand_0 /bin/sh -c 'drand share --connect drand_3:8380 --tls-disable' &


sleep 10

## Test beacon
## Generate keys
nohup docker exec -u drand drand_0 /bin/sh -c 'drand generate-keypair --tls-disable --id test_beacon "drand_0:8080"' &
nohup docker exec -u drand drand_1 /bin/sh -c 'drand generate-keypair --tls-disable --id test_beacon "drand_1:8180"' &
nohup docker exec -u drand drand_2 /bin/sh -c 'drand generate-keypair --tls-disable --id test_beacon "drand_2:8280"' &
nohup docker exec -u drand drand_3 /bin/sh -c 'drand generate-keypair --tls-disable --id test_beacon "drand_3:8380"' &

# Start leader
nohup docker exec -u drand drand_3 /bin/sh -c 'drand share --leader --nodes 4 --threshold 3 --period "60s" --id test_beacon' &

sleep 3

# Start the rest of the nodes
nohup docker exec -u drand drand_2 /bin/sh -c 'drand share --connect drand_3:8380 --tls-disable --id test_beacon' &
nohup docker exec -u drand drand_1 /bin/sh -c 'drand share --connect drand_3:8380 --tls-disable --id test_beacon' &
nohup docker exec -u drand drand_0 /bin/sh -c 'drand share --connect drand_3:8380 --tls-disable --id test_beacon' &
