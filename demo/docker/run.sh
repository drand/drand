#!/bin/bash
set -e # exit on non-zero status code

# post-run cleanup
cleanup () {
  docker-compose kill
  docker-compose rm -f
}
trap 'cleanup ; printf "Tests have been killed via signal.\n"' HUP INT QUIT PIPE TERM

# nodes write in this shared file to build the common group
GROUP_FILE="./data/group.toml"

# threshold: 3 nodes out of 5, period for randomness: 10 seconds
echo -en "Threshold = 3\nPeriod = \"10s\"\n\n" > "${GROUP_FILE}" 
chmod ugo+rwx "${GROUP_FILE}" 
rm -f data/*.public

# build and run 5 docker images (what each container does is in data/client-script.sh)
docker-compose up -d --build

echo ""
echo "Congratulations! the drand network is running."
echo ""
echo "Query a node's API: "
echo "  Linux: curl CONTAINER_IP:PORT/api/public "
echo "  alternative: docker exec drand1 call_api "
echo ""

docker-compose logs -f
cleanup