#!/usr/bin/env bash

set -ex

pushd "$(git rev-parse --show-toplevel)"

echo "Building docker images"
make build_docker_all

popd

pushd "$(git rev-parse --show-toplevel)"/test/docker

echo "Creating data_% directories"
mkdir -p data_{0,1,2,3,4}
chmod 777 data_{0,1,2,3,4}

echo "Launching Drand Docker Compose"
docker-compose up -d

echo "" > nohup.out

./utils/startBeacon.sh

popd
