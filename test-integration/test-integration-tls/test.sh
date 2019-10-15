#!/bin/bash

DOCKER_COMPOSE_PROJECT_NAME="drand-integration-test"
DOCKER_COMPOSE_FILE="integration-test.yml"
GROUP_FILE="./data/group.toml"
LOG_FILE="./last_run.log"
RETRIES=10

# import helper functions
source ../utils.sh

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
rm -rf data/TLS_certificates
rm -rf data/TLS_privatekeys

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
RANDOM_CONTAINER_ID=$(random_drand_container_id "${DOCKER_COMPOSE_PROJECT_NAME}")
CONTAINER_WEB_API_ADDR=$(container_https_url_from_id "${RANDOM_CONTAINER_ID}")

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
RANDOM_CONTAINER_ID=$(random_drand_container_id "${DOCKER_COMPOSE_PROJECT_NAME}")
CONTAINER_WEB_API_ADDR=$(container_https_url_from_id "${RANDOM_CONTAINER_ID}")

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