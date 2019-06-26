#!/bin/sh

IP_ADDR=`ip a | grep -oE "\b([0-9]{1,3}\.){3}[0-9]{1,3}\b" | grep 172.33.0`
IP_ADDR_PORT="${IP_ADDR}:${PORT}"

echo "My IP is ${IP_ADDR_PORT}"
echo

# Generating key pair, adding it to the shared volume /data/group.toml
echo "Generating key pair..."
rm -rf /root/.drand/
drand generate-keypair "${IP_ADDR_PORT}"
echo "Here they are is:"
ls /root/.drand/key
PUBLIC_KEY=`cat /root/.drand/key/drand_id.public`
echo -e "${PUBLIC_KEY}\n" >> /data/group.toml
echo

# Wait for all containers to have done the same
sleep 10

drand check-group /data/group.toml

echo "Done"