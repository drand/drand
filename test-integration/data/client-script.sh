#!/bin/sh

IP_ADDR=`ip a | grep -oE "\b([0-9]{1,3}\.){3}[0-9]{1,3}\b" | grep 172.33.0`
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
drand generate-keypair "${IP_ADDR_PORT}"
cp /root/.drand/key/drand_id.public "${PUBLIC_KEY_FILE}"
chmod ugo+rwx "${PUBLIC_KEY_FILE}"

PUBLIC_KEY=`cat /root/.drand/key/drand_id.public`
echo -en "[[Nodes]]\n${PUBLIC_KEY}\n\n" >> "${GROUP_FILE}"
echo

# Generate the TLS certificates
GOROOT=$(go env GOROOT)
mkdir -p "${TLS_CERTS_FOLDER}"
chmod ugo+rwx "${TLS_CERTS_FOLDER}"
cd "${TLS_CERTS_FOLDER}"
go run $GOROOT/src/crypto/tls/generate_cert.go --host $host --rsa-bits 1024 > /dev/null 2>& 1

# Boot the drand deamon in background
nohup drand --verbose 2 start --tls-cert "${TLS_CERT}" --tls-key "${TLS_KEY}" &

# Wait for all containers to have done the same
sleep 5

if [[ "$LEADER" == 1 ]]; then
    sleep 5
    echo "We are the leader, running DKG"
    drand share --leader "${GROUP_FILE}"
else
    drand share "${GROUP_FILE}"
fi

sleep 15

echo "Done"