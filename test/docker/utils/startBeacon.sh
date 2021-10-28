#bin/sh

# Start leader
nohup docker exec -u drand drand_3 /bin/sh -c 'drand share --leader --nodes 4 --threshold 3 --period "5s"' &

sleep 3s

# Start the rest of the nodes
nohup docker exec -u drand drand_2 /bin/sh -c 'drand share --connect drand_3:8380 --tls-disable' &
nohup docker exec -u drand drand_1 /bin/sh -c 'drand share --connect drand_3:8380 --tls-disable' &
nohup docker exec -u drand drand_0 /bin/sh -c 'drand share --connect drand_3:8380 --tls-disable' &