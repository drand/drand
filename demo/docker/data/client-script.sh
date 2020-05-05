#!/bin/sh

# read this container's IP address
IP_ADDR=`ip a | grep global | grep -oE "\b([0-9]{1,3}\.){3}[0-9]{1,3}\b" | head -n 1`
IP_ADDR_PORT="${IP_ADDR}:${PORT}"

SHARED_FOLDER="/data"
PUBLIC_KEY_FILE="${SHARED_FOLDER}/${PORT}.public"
GROUP_FILE="${SHARED_FOLDER}/group.toml"

# Generate key pair, add it to the shared file /data/group.toml
echo "Generating key pair..."
rm -rf /root/.drand/
drand generate-keypair --tls-disable "${IP_ADDR_PORT}"
cp /root/.drand/key/drand_id.public "${PUBLIC_KEY_FILE}"
chmod ugo+rwx "${PUBLIC_KEY_FILE}"

PUBLIC_KEY=`cat /root/.drand/key/drand_id.public`
echo -en "[[Nodes]]\n${PUBLIC_KEY}\n\n" >> "${GROUP_FILE}"
echo

# On MacOS, you can't ping/curl the container from the host. Here's a fix
apk add curl
cat << FIX_SCRIPT > /bin/call_api
#!/bin/sh
echo "Running curl -s ${IP_ADDR_PORT}/api/public from inside the container:"
curl -s ${IP_ADDR_PORT}/api/public
FIX_SCRIPT
chmod ug+x /bin/call_api

# Boot the drand deamon in background
nohup drand start --tls-disable & # add "--verbose 2" here for more details

# Wait for all containers to have done the same
sleep 5

# Now nodes wait for the leader to run DKG; leader starts DKG
if [[ "$LEADER" == 1 ]]; then
    sleep 5
    echo "We are the leader, checking group..."
    drand check-group "${GROUP_FILE}"
    echo

    echo "Running DKG..."
    drand share --leader "${GROUP_FILE}"
else
    drand share "${GROUP_FILE}"
fi

# Let the deamon alive for long enough
sleep 9999

echo "Done"