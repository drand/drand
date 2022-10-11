## Instructions

---
1) Build the image from docker image with `docker build --build-arg version=$(git describe --tags) --build-arg gitCommit=$(git rev-parse HEAD) -t drandorg/go-drand:latest .` in the project root folder
2) Run `docker compose -f "docker-compose.yml" up`
3) Execute the script called `startBeacon.sh` in order to run the setup phase. After that, the nodes will start to generate randomness
