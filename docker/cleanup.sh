#!/bin/sh

docker compose -f ./docker-compose-network.yml down
docker stop $(docker ps --all --quiet)
docker rm $(docker ps --all --quiet)
docker volume rm $(docker volume ls --quiet)
