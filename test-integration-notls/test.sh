#!/bin/sh

DOCKER_COMPOSE_PROJECT_NAME="drand-integration-test"
DOCKER_COMPOSE_FILE="integration-test.yml"
GROUP_FILE="./data/group.toml"

# post-test cleanup
cleanup () {
  docker-compose -p "${DOCKER_COMPOSE_PROJECT_NAME}" -f "${DOCKER_COMPOSE_FILE}" kill
  docker-compose -p "${DOCKER_COMPOSE_PROJECT_NAME}" -f "${DOCKER_COMPOSE_FILE}" rm -f
}
trap 'cleanup ; printf "Tests have been killed via signal.\n"' HUP INT QUIT PIPE TERM

# clear the previous group
echo -en "Threshold = 3\nPeriod = \"1m0s\"\n\n" > "${GROUP_FILE}" 
chmod ugo+rwx "${GROUP_FILE}" 
rm -f data/*.public
rm -rf data/TLS_certificates_*

# build and run the composed services
docker-compose -p "${DOCKER_COMPOSE_PROJECT_NAME}" -f "${DOCKER_COMPOSE_FILE}" up -d # add "--build" here
if [ $? -ne 0 ] ; then
  printf "Docker-compose drand build+startup failed.\n"
  exit -1
fi

docker-compose -p "${DOCKER_COMPOSE_PROJECT_NAME}" -f "${DOCKER_COMPOSE_FILE}" logs -f

echo "Gonna test for web server API..."

sleep 1000

cleanup
exit 0


# wait for the test service to complete and grab the exit code
#TEST_EXIT_CODE=`docker wait ci_integration-tester_1`

# inspect the output of the test and display respective message
if [ -z ${TEST_EXIT_CODE+x} ] || [ "$TEST_EXIT_CODE" -ne 0 ] ; then
  printf "${RED}Tests Failed${NC} - Exit Code: $TEST_EXIT_CODE\n"
else
  printf "${GREEN}Tests Passed${NC}\n"
fi

cleanup
exit 0
