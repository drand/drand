#bin/sh

# TODO Previous tasks
# TODO Run dkg first with startBeacon.sh script
# TODO Copy group file .drand/groups/drand_group.toml from node 0 to node 4 volume
# TODO You are ready to run this script

# Start leader
nohup docker exec -u drand drand_0 /bin/sh -c 'drand share --transition --leader --nodes 5 --threshold 4 --period "5"' &

sleep 3s

# Start the rest of the nodes
nohup docker exec -u drand drand_2 /bin/sh -c 'drand share --transition  --connect drand_0:8080 --tls-disable' &
nohup docker exec -u drand drand_1 /bin/sh -c 'drand share --transition  --connect drand_0:8080 --tls-disable' &
nohup docker exec -u drand drand_3 /bin/sh -c 'drand share --transition  --connect drand_0:8080 --tls-disable' &
nohup docker exec -u drand drand_4 /bin/sh -c 'drand share --connect drand_0:8080 --from ./data/drand/.drand/groups/drand_group.toml --tls-disable' &