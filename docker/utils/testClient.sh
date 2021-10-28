################ Insecure mode ################
###############################################

# Insecure is used because we are not using TLS to initiate the communication and we are not providing chain hash nor group file

# Get last random value over http
docker exec -u drand drand_0 /bin/sh -c 'drand-client --url http://drand_0:8081 --insecure'
docker exec -u drand drand_1 /bin/sh -c 'drand-client --url http://drand_1:8181 --insecure'
docker exec -u drand drand_2 /bin/sh -c 'drand-client --url http://drand_2:8281 --insecure'

# Get last random value over grpc
docker exec -u drand drand_0 /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure'
docker exec -u drand drand_1 /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure'
docker exec -u drand drand_2 /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure'


# Get random values as they become available over grpc
docker exec -u drand drand_0 /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure --watch'
docker exec -u drand drand_1 /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure --watch'
docker exec -u drand drand_2 /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure --watch'

# Get random values generated on round 1050 over grpc
docker exec -u drand drand_0 /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure --round 1050'
docker exec -u drand drand_1 /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure --round 1050'
docker exec -u drand drand_2 /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure --round 1050'

# Get random values generated on round 1050 over http
docker exec -u drand drand_0 /bin/sh -c 'drand-client --url http://drand_0:8081 --insecure --round 1050'
docker exec -u drand drand_1 /bin/sh -c 'drand-client --url http://drand_1:8181 --insecure --round 1050'
docker exec -u drand drand_2 /bin/sh -c 'drand-client --url http://drand_2:8281 --insecure --round 1050'

############### With chain hash ###############
###############################################

# Insecure is kept there because we are not using TLS to initiate the communication

# Get last random value over http
docker exec -u drand drand_0 /bin/sh -c 'drand-client --url http://drand_0:8081 --insecure --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84'
docker exec -u drand drand_1 /bin/sh -c 'drand-client --url http://drand_1:8181 --insecure --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84'
docker exec -u drand drand_2 /bin/sh -c 'drand-client --url http://drand_2:8281 --insecure --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84'

# Get last random value over grpc
docker exec -u drand drand_0 /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84'
docker exec -u drand drand_1 /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84'
docker exec -u drand drand_2 /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84'


# Get random values as they become available over grpc
docker exec -u drand drand_0 /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure --watch --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84'
docker exec -u drand drand_1 /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure --watch --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84'
docker exec -u drand drand_2 /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure --watch --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84'

# Get random values generated on round 1050 over grpc
docker exec -u drand drand_0 /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure --round 1050 --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84'
docker exec -u drand drand_1 /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure --round 1050 --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84'
docker exec -u drand drand_2 /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure --round 1050 --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84'

# Get random values generated on round 1050 over http
docker exec -u drand drand_0 /bin/sh -c 'drand-client --url http://drand_0:8081 --insecure --round 1050 --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84'
docker exec -u drand drand_1 /bin/sh -c 'drand-client --url http://drand_1:8181 --insecure --round 1050 --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84'
docker exec -u drand drand_2 /bin/sh -c 'drand-client --url http://drand_2:8281 --insecure --round 1050 --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84'

############### With chain hash #################
################ and config file ################

# Insecure is kept there because we are not using TLS to initiate the communication

# Get last random value over http
docker exec -u drand drand_0 /bin/sh -c 'drand-client --url http://drand_0:8081 --insecure --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84 --group-conf ./data/drand/.drand/multibeacon/default/groups/drand_group.toml'
docker exec -u drand drand_1 /bin/sh -c 'drand-client --url http://drand_1:8181 --insecure --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84 --group-conf ./data/drand/.drand/multibeacon/default/groups/drand_group.toml'
docker exec -u drand drand_2 /bin/sh -c 'drand-client --url http://drand_2:8281 --insecure --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84 --group-conf ./data/drand/.drand/multibeacon/default/groups/drand_group.toml'

# Get last random value over grpc
docker exec -u drand drand_0 /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84 --group-conf ./data/drand/.drand/multibeacon/default/groups/drand_group.toml'
docker exec -u drand drand_1 /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84 --group-conf ./data/drand/.drand/multibeacon/default/groups/drand_group.toml'
docker exec -u drand drand_2 /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84 --group-conf ./data/drand/.drand/multibeacon/default/groups/drand_group.toml'


# Get random values as they become available over grpc
docker exec -u drand drand_0 /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure --watch --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84 --group-conf ./data/drand/.drand/multibeacon/default/groups/drand_group.toml'
docker exec -u drand drand_1 /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure --watch --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84 --group-conf ./data/drand/.drand/multibeacon/default/groups/drand_group.toml'
docker exec -u drand drand_2 /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure --watch --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84 --group-conf ./data/drand/.drand/multibeacon/default/groups/drand_group.toml'

# Get random values generated on round 1050 over grpc
docker exec -u drand drand_0 /bin/sh -c 'drand-client --grpc-connect drand_0:8080 --insecure --round 1050 --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84 --group-conf ./data/drand/.drand/multibeacon/default/groups/drand_group.toml'
docker exec -u drand drand_1 /bin/sh -c 'drand-client --grpc-connect drand_1:8180 --insecure --round 1050 --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84 --group-conf ./data/drand/.drand/multibeacon/default/groups/drand_group.toml'
docker exec -u drand drand_2 /bin/sh -c 'drand-client --grpc-connect drand_2:8280 --insecure --round 1050 --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84 --group-conf ./data/drand/.drand/multibeacon/default/groups/drand_group.toml'

# Get random values generated on round 1050 over http
docker exec -u drand drand_0 /bin/sh -c 'drand-client --url http://drand_0:8081 --insecure --round 1050 --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84 --group-conf ./data/drand/.drand/multibeacon/default/groups/drand_group.toml'
docker exec -u drand drand_1 /bin/sh -c 'drand-client --url http://drand_1:8181 --insecure --round 1050 --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84 --group-conf ./data/drand/.drand/multibeacon/default/groups/drand_group.toml'
docker exec -u drand drand_2 /bin/sh -c 'drand-client --url http://drand_2:8281 --insecure --round 1050 --chain-hash dd163beccd21f61182a5153df247a10b9386c27803b498c1febdc3337416fd84 --group-conf ./data/drand/.drand/multibeacon/default/groups/drand_group.toml'

