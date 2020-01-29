# drand Demo

Prerequisites:

1. `docker >= 17.12`
2. `docker-compose >= 1.18`

Run the demo: `./run.sh`

This commands builds the docker images, and starts the containers: 

![Setup](https://user-images.githubusercontent.com/5019664/73373513-06e0cc00-42b9-11ea-9a87-3acf16a20b29.png)

... then, the nodes start generating randomness periodically:

![Randomness](https://user-images.githubusercontent.com/5019664/73373726-5e7f3780-42b9-11ea-8821-27146ed1e701.png)

You may inspect the entrypoint of the clients in `/data/client-script.sh`.

## Manually contacting the API

drand has a Web REST api which can be contacted by `curl`:

`curl CONTAINER_IP:PORT/api/public`, where `PORT` is `8080-8085` in this demo.

However, on MacOS X, `curl` cannot contact internal containers (see the [issue](https://github.com/drand/drand/pull/193)); instead, please run

`docker exec drand1 call_api`

which simply calls `curl` from inside `drand1`.