#!/usr/bin/env bash

docker compose -f ./docker-compose-nginx.yml down --volumes --remove-orphans
docker compose -f ./docker-compose-network.yml down --volumes --remove-orphans

volumes=$(docker volume ls  --format '{{.Name}}' | grep -i drand_docker_demo | tr '\n' ' ')

if [[ -n "$volumes" ]]; then
  docker volume rm $volumes
fi


