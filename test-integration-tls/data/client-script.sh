#!/bin/sh

IP_ADDR=`ip a | grep global | grep -oE "\b([0-9]{1,3}\.){3}[0-9]{1,3}\b" | head -n 1`
IP_ADDR_PORT="${IP_ADDR}:${PORT}"

SHARED_FOLDER="/data"
PUBLIC_KEY_FILE="${SHARED_FOLDER}/${PORT}.public"
GROUP_FILE="${SHARED_FOLDER}/group.toml"
TLS_KEY_FOLDER="${SHARED_FOLDER}/TLS_privatekeys"
TLS_KEY="${TLS_KEY_FOLDER}/key${PORT}.pem"
TLS_CERT_FOLDER="${SHARED_FOLDER}/TLS_certificates"
TLS_CERT="${TLS_CERT_FOLDER}/cert${PORT}.pem"


echo "My IP is ${IP_ADDR_PORT}"
echo

# Generating key pair, adding it to the shared volume /data/group.toml
echo "Generating key pair..."
rm -rf /root/.drand/
drand generate-keypair "${IP_ADDR_PORT}"
cp /root/.drand/key/drand_id.public "${PUBLIC_KEY_FILE}"
chmod ugo+rwx "${PUBLIC_KEY_FILE}"

PUBLIC_KEY=`cat /root/.drand/key/drand_id.public`
echo -en "[[Nodes]]\n${PUBLIC_KEY}\n\n" >> "${GROUP_FILE}"
echo

# Generate the TLS certificates
GOROOT=$(go env GOROOT)
mkdir -p "${TLS_KEY_FOLDER}"
chmod ugo+rwx "${TLS_KEY_FOLDER}"
mkdir -p "${TLS_CERT_FOLDER}"
chmod ugo+rwx "${TLS_CERT_FOLDER}"
go run $GOROOT/src/crypto/tls/generate_cert.go --host "${IP_ADDR}" --rsa-bits 1024 > /dev/null 2>& 1
chmod ugo+rwx *.pem
cp "cert.pem" "${TLS_CERT}"
cp "key.pem" "${TLS_KEY}"

# Wait for all containers to have done the same
sleep 5

# Boot the drand deamon in background
nohup drand --verbose 2 start  --certs-dir "${TLS_CERT_FOLDER}" --tls-cert "${TLS_CERT}" --tls-key "${TLS_KEY}" &

# Wait for all containers to have done the same
sleep 5

# Now nodes wait for the leader to run DKG; leader starts DKG
if [[ "$LEADER" == 1 ]]; then
    sleep 5
    echo "We are the leader, checking group..."
    drand check-group  --certs-dir "${TLS_CERT_FOLDER}" "${GROUP_FILE}"
    echo

    echo "Running DKG..."
    drand share --leader "${GROUP_FILE}"
else
    drand share "${GROUP_FILE}"
fi

# Let the deamon alive for long enough
sleep 9999

echo "Done"