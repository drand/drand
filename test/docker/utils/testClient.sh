################ Insecure mode ################
###############################################

# Insecure is used because we are not using TLS to initiate the communication and we are not providing chain hash nor group file

# Get last random value over http
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_0:8081 --insecure'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_1:8181 --insecure'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_2:8281 --insecure'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_3:8381 --insecure'

# Get last random value over grpc
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_3:8380 --insecure'


# Get random values as they become available over grpc
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure --watch'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure --watch'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure --watch'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_3:8380 --insecure --watch'

# Get random values generated on round 1050 over grpc
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure --round 1050'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure --round 1050'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure --round 1050'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_3:8380 --insecure --round 1050'

# Get random values generated on round 1050 over http
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_0:8081 --insecure --round 1050'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_1:8181 --insecure --round 1050'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_2:8281 --insecure --round 1050'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_3:8381 --insecure --round 1050'

############### With chain hash ###############
###############################################

# Insecure is kept there because we are not using TLS to initiate the communication

# Get last random value over http
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_0:8081 --insecure --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_1:8181 --insecure --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_2:8281 --insecure --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_2:8281 --insecure --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'

# Get last random value over grpc
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_3:8380 --insecure --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'


# Get random values as they become available over grpc
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure --watch --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure --watch --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure --watch --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_3:8380 --insecure --watch --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'

# Get random values generated on round 1050 over grpc
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure --round 1050 --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure --round 1050 --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure --round 1050 --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_3:8380 --insecure --round 1050 --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'

# Get random values generated on round 1050 over http
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_0:8081 --insecure --round 1050 --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_1:8181 --insecure --round 1050 --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_2:8281 --insecure --round 1050 --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_2:8281 --insecure --round 1050 --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493'

############### With chain hash #################
################ and config file ################

# Insecure is kept there because we are not using TLS to initiate the communication

# Get last random value over http
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_0:8081 --insecure --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_0/.drand/multibeacon/default/groups/drand_group.toml'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_1:8181 --insecure --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_1/.drand/multibeacon/default/groups/drand_group.toml'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_2:8281 --insecure --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_2/.drand/multibeacon/default/groups/drand_group.toml'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_2:8281 --insecure --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_3/.drand/multibeacon/default/groups/drand_group.toml'

# Get last random value over grpc
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_0/.drand/multibeacon/default/groups/drand_group.toml'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_1/.drand/multibeacon/default/groups/drand_group.toml'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_2/.drand/multibeacon/default/groups/drand_group.toml'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_3:8380 --insecure --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_3/.drand/multibeacon/default/groups/drand_group.toml'


# Get random values as they become available over grpc
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure --watch --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_0/.drand/multibeacon/default/groups/drand_group.toml'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure --watch --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_1/.drand/multibeacon/default/groups/drand_group.toml'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure --watch --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_2/.drand/multibeacon/default/groups/drand_group.toml'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_3:8380 --insecure --watch --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_3/.drand/multibeacon/default/groups/drand_group.toml'

# Get random values generated on round 1050 over grpc
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure --round 1050 --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_0/.drand/multibeacon/default/groups/drand_group.toml'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure --round 1050 --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_1/.drand/multibeacon/default/groups/drand_group.toml'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure --round 1050 --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_2/.drand/multibeacon/default/groups/drand_group.toml'
docker exec drand_client /bin/sh -c 'drand-client --grpc-connect drand_3:8380 --insecure --round 1050 --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_3/.drand/multibeacon/default/groups/drand_group.toml'

# Get random values generated on round 1050 over http
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_0:8081 --insecure --round 1050 --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_0/.drand/multibeacon/default/groups/drand_group.toml'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_1:8181 --insecure --round 1050 --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_1/.drand/multibeacon/default/groups/drand_group.toml'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_2:8281 --insecure --round 1050 --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_2/.drand/multibeacon/default/groups/drand_group.toml'
docker exec drand_client /bin/sh -c 'drand-client --url http://drand_3:8381 --insecure --round 1050 --chain-hash 945ae851f30772add04b090fd6ba3d741969e38eee2f26fc77533e0d20a90493 --group-conf ./data/drand_3/.drand/multibeacon/default/groups/drand_group.toml'
