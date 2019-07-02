#!/bin/sh

DOCKER_COMPOSE_PROJECT_NAME="drand-integration-test"
DOCKER_COMPOSE_NETWORK_NAME="drand-network"
DOCKER_COMPOSE_FILE="integration-test.yml"
GROUP_FILE="./data/group.toml"
LOG_FILE="./last_run.log"
RETRIES=10

# utility functions
json_web_request () {
  ADDRESS=$1
  curl -s -S -H "Content-type: application/json" "${ADDRESS}"
}

container_ip_and_port_from_id() {
  ID=$1
  CONTAINER_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "${RANDOM_CONTAINER_ID}")
  CONTAINER_PORT=$(docker inspect --format='{{range $p, $conf := .NetworkSettings.Ports}}{{(index $conf 0).HostPort}}{{end}}' "${RANDOM_CONTAINER_ID}")
  echo "http://${CONTAINER_IP}:${CONTAINER_PORT}/api/"
}

test_and_retrieve_keys_and_randomness() {
  URL=$1

  # Retrieve Randomness
  API_ANSWER2=$(json_web_request "${URL}public")

  echo "Distributed Randomness:"
  echo "${API_ANSWER2}" | jq
  echo ""

  echo "${API_ANSWER}" | jq -e ".randomness" >/dev/null
  if [ $? -ne 0 ]; then
    echo "Test failed, couldn't fetch randomness."
    exit 1
  fi

  # Retrieve Public Key
  API_ANSWER3=$(json_web_request "${URL}info/distkey")
  echo "Distributed Public Key:"
  echo "${API_ANSWER3}" | jq
  echo ""

  echo "${API_ANSWER3}" | jq -e ".key" >/dev/null
  if [ $? -ne 0 ]; then
    echo "Test failed, couldn't fetch public key."
    exit 1
  fi

  # Retrieve Group Info
  API_ANSWER4=$(json_web_request "${URL}info/group")
  echo "Group Info:"
  echo "${API_ANSWER4}" | jq
  echo ""

  echo "${API_ANSWER4}" | jq -e ".distkey" >/dev/null
  if [ $? -ne 0 ]; then
    echo "Test failed, couldn't fetch group info."
    exit 1
  fi
}

# post-test cleanup
cleanup () {
  docker-compose -p "${DOCKER_COMPOSE_PROJECT_NAME}" kill
  docker-compose -p "${DOCKER_COMPOSE_PROJECT_NAME}" rm -f
}
trap 'cleanup ; printf "Tests have been killed via signal.\n"' HUP INT QUIT PIPE TERM

# clear the previous group
echo -en "Threshold = 3\nPeriod = \"10s\"\n\n" > "${GROUP_FILE}" 
chmod ugo+rwx "${GROUP_FILE}" 
rm -f data/*.public
rm -rf data/TLS_certificates_*

# build and run the composed services$
echo "Building the containers..."
echo ""
docker-compose -p "${DOCKER_COMPOSE_PROJECT_NAME}" up -d --build
if [ $? -ne 0 ] ; then
  printf "Docker-compose drand build+startup failed.\n"
  exit -1
fi

# redirect logs to LOG_FILE
rm -f "${LOG_FILE}"
docker-compose -p "${DOCKER_COMPOSE_PROJECT_NAME}" logs -f > "${LOG_FILE}" & # this is magically killed when the containers go down. Neat!

echo ""
echo "Letting the containers boot (10 seconds)..."
sleep 10

# find the name of any container
RANDOM_CONTAINER_ID=$(docker network inspect -f '{{ range $key, $value := .Containers }}{{ printf "%s\n" $key }}{{ end }}' "${DOCKER_COMPOSE_PROJECT_NAME}_${DOCKER_COMPOSE_NETWORK_NAME}" | head -n 1)
CONTAINER_WEB_API_ADDR=$(container_ip_and_port_from_id "${RANDOM_CONTAINER_ID}")

# 10 attempts to fetch randomness from this container
echo "Contacting container ${RANDOM_CONTAINER_ID} on ${CONTAINER_WEB_API_ADDR}public to get public randomness"
for REPEAT in $(seq 0 $RETRIES)
do
  API_ANSWER=$(json_web_request "${CONTAINER_WEB_API_ADDR}public")

  echo "${API_ANSWER}" | jq -e ".error" >/dev/null
  if [ $? -eq 0 ]; then # then field "error" exists"
    echo "Error: ${API_ANSWER}"
    echo "Sleeping 10 and retrying ($REPEAT/$RETRIES)..."
    sleep 10

    if [ "${REPEAT}" == "${RETRIES}" ]; then
      echo "Couldn't fetch randomness, aborting."
      exit 1
    fi
  else
    break
  fi
done
echo ""

test_and_retrieve_keys_and_randomness "${CONTAINER_WEB_API_ADDR}"

echo "Now testing with one node down..."
docker stop "${RANDOM_CONTAINER_ID}"
echo "... node shut down. Waiting for the new randomness..."
sleep 20 # 2*period

# now test again to fetch some randomness
RANDOM_CONTAINER_ID=$(docker network inspect -f '{{ range $key, $value := .Containers }}{{ printf "%s\n" $key }}{{ end }}' "${DOCKER_COMPOSE_PROJECT_NAME}_${DOCKER_COMPOSE_NETWORK_NAME}" | head -n 1)
CONTAINER_WEB_API_ADDR=$(container_ip_and_port_from_id "${RANDOM_CONTAINER_ID}")

echo "Contacting container ${RANDOM_CONTAINER_ID} on ${CONTAINER_WEB_API_ADDR}public to get public randomness"
for REPEAT in $(seq 0 $RETRIES)
do
  API_ANSWER=$(json_web_request "${CONTAINER_WEB_API_ADDR}public")

  echo "${API_ANSWER}" | jq -e ".error" >/dev/null
  if [ $? -eq 0 ]; then # then field "error" exists"
    echo "Error: ${API_ANSWER}"
    echo "Sleeping 10 and retrying ($REPEAT/$RETRIES)..."
    sleep 10

    if [ "${REPEAT}" == "${RETRIES}" ]; then
      echo "Couldn't fetch randomness, aborting."
      exit 1
    fi
  else
    break
  fi
done
echo ""

test_and_retrieve_keys_and_randomness "${CONTAINER_WEB_API_ADDR}"

echo "Test finished successfully, cleaning up..."

cleanup
exit 0