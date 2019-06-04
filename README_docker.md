# Drand on Docker

The main readme is [here](./README.md).

## Simple Docker & Docker-compose setup

Creating a docker image from scratch:

Let's write a `Dockerfile` with the following contents:

	FROM golang:1.12.5-alpine # update this number from time to time, but keep "alpine" for a lightweight image
	RUN apk update
	RUN apk upgrade
	RUN apk add git
	RUN go get -u github.com/dedis/drand
	WORKDIR /
	ENTRYPOINT ["drand", "--verbose", "2", "generate-keys"]

This image COULD already be built and run via ` docker build - < Dockerfile`, if you prefer not to use docker-compose. But I wouldn't recommand it, so let's continue with docker-compose. It will help keeping our container running, plus manage volumes, etc.

Create the following `docker-compose.yml` :

	drand:
	  build: .
	  volumes:
	    - ./data:/root/.drand
	  ports:
	    - "0.0.0.0:1234:8080" # replace 1234 with the external port you want

Finally, also create a `data` folder to hold the keys and settings. It has to have permissions `740`

	mkdir data
	chmod 740 data

Drand can then be built into an image with

	docker-compose up --build

This command should have generated you keys in `data/keys`. If that's the case, we can now kill the image with `CTRL-C`, then change the entrypoint in `Dockerfile` with

	ENTRYPOINT ["drand", "--verbose", "2", "start", "--listen", "0.0.0.0:8080"]

and the image is finally ready for DKG, just start it and let it run in background with 

	docker-compose up -d

(one can also set a restart policy in the docker-compose.yml)

## DKG

If you did the setup above, you have a container running, loaded with your keys. It still misses two things:

1. the group.toml file corresponding to other participants. For this, you have to exchange keys manually.
2. running the DKG protocol to bootstrap drand

Fortunately, with our docker-compose volumes, it's now very easy. Just added your `group.toml` into the root of the `data` folder.

Then, open a CLI into your running docker by locating its id:

	697e4766f8b2        drand_drand             "drand --verbose 2 s…"   11 minutes ago      Up 9 minutes

The id of the container is `697e4766f8b2`. Enter it by running

	docker exec -it 697e4766f8b2 /bin/sh

Then, you're inside the container; tell drand to run the DKG like so:

	drand share /root/.drand/group.toml

__Notice the full path__ `/root/.drand/group.toml` and not `group.toml` nor `./group.toml`

## Checking the logs

From outside the containers, run `docker-compose logs`

## Updating drand

To update the docker, simply shut it down

	docker-compose down

and to fully rebuild it, you need to first clean the already-used layers (for some reason Docker is confused and thinks nothing has changed, and it keeps rebuilding the old version)

	docker system prune -a

Caution! this delete *all* your *unused* containers and networks; it's typically fine, but just be aware of it.

Then rebuild and restart it

	docker-compose up --build -d

## Docker behind reverse proxy setup

Typically, I only have TLS certificates for the reverse proxy in my VPS, and hence my local `drand` needs to use the `--tls-disable` flag, like so in the `Dockerfile`:

	ENTRYPOINT ["drand", "--verbose", "2", "start", "--tls-disable", "--listen", "0.0.0.0:8080"]

*But* notice that to others, you'll still be using TLS (handled by your reverse proxy), so make sure you generate keys using an https address, and the flag `TLS=true`.