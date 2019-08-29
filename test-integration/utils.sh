#!/bin/sh

# Note: this file only contains utility functions. The main logic is not here

json_web_request () {
  ADDRESS=$1
  curl -s -S -k -H "Content-type: application/json" "${ADDRESS}"
}

get_docker_drand_network_name() {
    DRAND_NETWORK_ID=$(docker network ls | grep drand-networ | cut -d " " -f 1)
    DRAND_NETWORK_NAME=$(docker inspect -f '{{.Name}}' "${DRAND_NETWORK_ID}")
    echo "${DRAND_NETWORK_NAME}"
}

container_ip_port_from_id() {
  CONTAINER_ID=$1
  CONTAINER_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "${CONTAINER_ID}")
  CONTAINER_PORT=$(docker inspect --format='{{range $p, $conf := .NetworkSettings.Ports}}{{(index $conf 0).HostPort}}{{end}}' "${CONTAINER_ID}")
  echo "${CONTAINER_IP}:${CONTAINER_PORT}"
}

container_http_url_from_id() {
  CONTAINER_ID=$1
  CONTAINER_IP_PORT=$(container_ip_port_from_id "${CONTAINER_ID}")
  echo "http://${CONTAINER_IP_PORT}/api/"
}

container_https_url_from_id() {
  CONTAINER_ID=$1
  CONTAINER_IP_PORT=$(container_ip_port_from_id "${CONTAINER_ID}")
  echo "https://${CONTAINER_IP_PORT}/api/"
}

random_drand_container_id() {
    DOCKER_COMPOSE_PROJECT_NAME=$1
    DOCKER_COMPOSE_NETWORK_NAME=$(get_docker_drand_network_name)
    RANDOM_CONTAINER_ID=$(docker network inspect -f '{{ range $key, $value := .Containers }}{{ printf "%s\n" $key }}{{ end }}' "${DOCKER_COMPOSE_NETWORK_NAME}" | head -n 1)
    echo "${RANDOM_CONTAINER_ID}"
}

test_and_retrieve_keys_and_randomness() {
  URL=$1

  # Retrieve Randomness
  API_ANSWER2=$(json_web_request "${URL}public")

  echo "Distributed Randomness:"
  echo "${API_ANSWER2}" | jq '.'
  echo ""

  echo "${API_ANSWER}" | jq -e ".randomness" >/dev/null
  if [ $? -ne 0 ]; then
    echo "Test failed, couldn't fetch randomness."
    exit 1
  fi

  # Retrieve Public Key
  API_ANSWER3=$(json_web_request "${URL}info/distkey")
  echo "Distributed Public Key:"
  echo "${API_ANSWER3}" | jq '.'
  echo ""

  echo "${API_ANSWER3}" | jq -e ".key" >/dev/null
  if [ $? -ne 0 ]; then
    echo "Test failed, couldn't fetch public key."
    exit 1
  fi

  # Retrieve Group Info
  API_ANSWER4=$(json_web_request "${URL}info/group")
  echo "Group Info:"
  echo "${API_ANSWER4}" | jq '.'
  echo ""

  echo "${API_ANSWER4}" | jq -e ".distkey" >/dev/null
  if [ $? -ne 0 ]; then
    echo "Test failed, couldn't fetch group info."
    exit 1
  fi
}
