#!/bin/bash

DOCKER_COMPOSE_PROJECT_NAME="drand-test"
GROUP_FILE="./data/group.toml" # nodes write in this shared file to build the common group

# post-run cleanup
cleanup () {
  docker-compose -p "${DOCKER_COMPOSE_PROJECT_NAME}" kill
  docker-compose -p "${DOCKER_COMPOSE_PROJECT_NAME}" rm -f
}
trap 'cleanup ; printf "Tests have been killed via signal.\n"' HUP INT QUIT PIPE TERM

# threshold: 3 nodes out of 5, period for randomness: 10 seconds
echo -en "Threshold = 3\nPeriod = \"10s\"\n\n" > "${GROUP_FILE}" 
chmod ugo+rwx "${GROUP_FILE}" 
rm -f data/*.public

# build and run 5 docker images (what each container does is in data/client-script.sh)
docker-compose -p "${DOCKER_COMPOSE_PROJECT_NAME}" up -d --build
docker-compose -p "${DOCKER_COMPOSE_PROJECT_NAME}" logs -f