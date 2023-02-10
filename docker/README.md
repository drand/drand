# Running a node with Docker

## Prerequisites
- install docker
- install docker compose (probably bundled with docker these days)
- run on linux :) (other platforms may work but YMMV)

## Creation of a keypair

Pull the latest drand image:
`docker pull drandorg/go-drand:v1.5.2-testnet`

Create a volume where you're going to store your keypairs and other config data
`docker volume create drand`

Next we must create a keypair and store it in the docker volume we've just created.

`docker run --volume drand:/data/drand drandorg/go-drand:v1.5.2-testnet generate-keypair  --folder /data/drand/.drand --tls-disable --id default 0.0.0.0:8080`

This will create a keypair for the default public listening address (0.0.0.0:8080) and store it in the `/data/drand/.drand` directory which is mapped to the `drand` volume we created in the previous step. You should replace 0.0.0.0:8080 with your public IP address, e.g. pl1-rpc.drand.sh:443, as this key is how other nodes in the network will verify that they're talking to your node. 
Note: access to this path should be firewalled to only allow connections from nodes in the relevant allowlist ([mainnet allowlist](https://github.com/drand/loe-mainnet-allowlist/) and [testnet allowlist](https://github.com/drand/loe-testnet-allowlist/))


## Starting drand
Finally we can start the docker container by running:
`docker run -d -p"8080:8080" -p"8888:8888" --name drand  --volume drand:/data/drand drandorg/go-drand:v1.5.2-testnet start --tls-disable --private-listen 0.0.0.0:8080`

If we run `docker logs -f drand`, we should be able to see that the node has started and is waiting for distributed key generation:
```
drand 1.5.2-testnet (date 06/02/2023@10:47:55, commit 7ea4a3c536bebf9436bfbc5d7b91eab9db932f53)
2023-02-08T14:08:42.622Z	INFO	0.0.0.0:8080	core/drand_daemon.go:117		{"network": "init", "insecure": true}
2023-02-08T14:08:42.643Z	INFO	0.0.0.0:8080	core/drand_daemon.go:145	DrandDaemon initialized	{"private_listen": "0.0.0.0:8080", "control_port": "8888", "public_listen": "", "folder": "/data/drand/.drand/multibeacon"}
2023-02-08T14:08:42.679Z	INFO	0.0.0.0:8080	core/drand_daemon.go:316	beacon id [default]: will run as fresh install -> expect to run DKG.
```

If we try and run `curl -v localhost:8080/chains` we won't get anything back! The private listening port only speaks gRPC and is used when nodes talk to one another. To expose the randomness itself, we must provide a public listening port.
Kill the container and rerun it with a command such as:
`docker run -d -p"8080:8080" -p"8888:8888" -p"9080:9080" --name drand  --volume drand:/data/drand drandorg/go-drand:v1.5.2-testnet start --tls-disable --private-listen 0.0.0.0:8080 --public-listen 0.0.0.0:9080`

Now if we run `curl -v localhost:9080/chains` we should get a 200 response back and an empty list of chains.

## Running with docker compose
This dir contains a sample [docker-compose.yml](./docker-compose.yml) that can be used to spin up a single node. You will still have to go through the steps of creating a volume and keypair above to use it.
Additionally, you can easily set up a test network of three nodes by running [the start-network.sh script](./start-network.sh). It can be torn down and cleaned up by using the [./cleanup.sh shell script](./cleanup.sh). This manifest will spin up a network of three nodes and run an initial distributed key generation process, and they will start generating randomness beacons.

## Running with nginx
Many LoE partners like to run a reverse proxy in front of their node to easily manage TLS termination, domain names and firewalling. 
In [docker-compose-nginx.yml](./docker-compose-nginx.yml) you can find a manifest for running a single drand docker container and an nginx container to route traffic to it. Similar to the keypair, we will have to create a volume containing the nginx config (and any TLS config you wish to add).