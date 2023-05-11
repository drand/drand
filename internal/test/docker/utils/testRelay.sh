## Start drand-relay-http inside running drand containers (Dockerfile)
mkdir -p tmp/relay/node_0
cd ./tmp/relay/node_0
nohup docker exec drand_relay /bin/sh -c 'drand-relay-http --url http://drand_0:8081 --insecure --bind 0.0.0.0:9080' &
cd ../../../

mkdir -p tmp/relay/node_1
cd ./tmp/relay/node_1
nohup docker exec drand_relay /bin/sh -c 'drand-relay-http --url http://drand_1:8181 --insecure --bind 0.0.0.0:9180' &
cd ../../../

mkdir -p tmp/relay/node_2
cd ./tmp/relay/node_2
nohup docker exec drand_relay /bin/sh -c 'drand-relay-http --url http://drand_2:8281 --insecure --bind 0.0.0.0:9280' &
cd ../../../

mkdir -p tmp/relay/node_3
cd ./tmp/relay/node_3
nohup docker exec drand_relay /bin/sh -c 'drand-relay-http --url http://drand_3:8381 --insecure --bind 0.0.0.0:9380' &
cd ../../../

sleep 5s

# Fetch round 1050
curl http://127.0.0.1:9080/public/1050
curl http://127.0.0.1:9180/public/1050
curl http://127.0.0.1:9280/public/1050

# Fetch latest round
curl http://127.0.0.1:9080/public/latest
curl http://127.0.0.1:9180/public/latest
curl http://127.0.0.1:9280/public/latest

# Fetch chain info
curl http://127.0.0.1:9080/info
curl http://127.0.0.1:9180/info
curl http://127.0.0.1:9280/info

# Check how relay is working (difference between last seen and expected round)
curl http://127.0.0.1:9080/health
curl http://127.0.0.1:9180/health
curl http://127.0.0.1:9280/health
