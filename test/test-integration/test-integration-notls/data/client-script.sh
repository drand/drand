#!/bin/sh

IP_ADDR=`ip a | grep global | grep -oE "\b([0-9]{1,3}\.){3}[0-9]{1,3}\b" | head -n 1`
IP_ADDR_PORT="${IP_ADDR}:${PORT}"

SHARED_FOLDER="/data"
PUBLIC_KEY_FILE="${SHARED_FOLDER}/${PORT}.public"
GROUP_FILE="${SHARED_FOLDER}/group.toml"
TLS_CERTS_FOLDER="${SHARED_FOLDER}/TLS_certificates_${PORT}/"
TLS_KEY="${TLS_CERTS_FOLDER}/key.pem"
TLS_CERT="${TLS_CERTS_FOLDER}/cert.pem"


echo "My IP is ${IP_ADDR_PORT}"
echo

# Generating key pair, adding it to the shared volume /data/group.toml
echo "Generating key pair..."
rm -rf /root/.drand/
drand generate-keypair --tls-disable "${IP_ADDR_PORT}"
cp /root/.drand/key/drand_id.public "${PUBLIC_KEY_FILE}"
chmod ugo+rwx "${PUBLIC_KEY_FILE}"

PUBLIC_KEY=`cat /root/.drand/key/drand_id.public`
echo -en "[[Nodes]]\n${PUBLIC_KEY}\n\n" >> "${GROUP_FILE}"
echo

# Boot the drand deamon in background
nohup drand --verbose 2 start --tls-disable &

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