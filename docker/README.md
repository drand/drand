# Running a node with Docker

## Prerequisites
- install docker
- install docker compose (probably bundled with your version of docker)
- run on linux :) (other platforms may work but YMMV)

## Creation of a keypair

Pull the latest drand image:
```shell
docker pull ghcr.io/drand/go-drand:latest
```
> [!NOTE]
> If you want to run drand locally without TLS, you should use the `ghcr.io/drand/go-drand-local:latest` image instead!

Create a volume where you're going to store your keypairs and other config data
```shell
docker volume create drand
```

Next we must create a keypair and store it in the docker volume we've just created.

```shell
docker run --rm --volume drand:/data/drand ghcr.io/drand/go-drand:latest generate-keypair  --folder /data/drand/.drand --id default 0.0.0.0:8080
```

> [!NOTE]
> If you are on mac M1/2/3 you will have to add `--platform linux/amd64` after the `run` but before the other arguments in all the commands

This will create a keypair for the default public listening address (0.0.0.0:8080) and store it in the `/data/drand/.drand` directory
which is mapped to the `drand` volume we created in the previous step.
> [!NOTE]
> An error such as 'Keys couldn't be loaded on drand daemon' is fine - this just means your daemon wasn't running while you generated your keys; it's possible to hot-load keys on a running daemon

> [!IMPORTANT]
> You should replace `0.0.0.0:8080` with your public IP address, e.g. `pl1-rpc.drand.sh:443`, as this key is how other nodes in the network
will verify that they're talking to your node.
> Access to this path should be firewalled to only allow connections from nodes in the relevant allowlist ([mainnet allowlist](https://github.com/drand/loe-mainnet-allowlist/) and [testnet allowlist](https://github.com/drand/loe-testnet-allowlist/))
> For League of Entropy members, there may be restrictions on what ports are accessible via others' security groups. Contact a member of the League of Entropy if you use an exotic port; 443/8080 should be fine.

> [!CAUTION]
> You should _not_ expose your control port to the internet nor other members of the network. If you do, they will be able to run arbitrary commands on your node and you will have a bad time.


## Starting drand
Finally we can start the docker container by running:
```shell
docker run --rm -d -p"8080:8080" -p"8888:8888" --name drand  --volume drand:/data/drand ghcr.io/drand/go-drand:latest start --private-listen 0.0.0.0:8080
```

If we run `docker logs -f drand`, we should be able to see that the node has started and is waiting for distributed key generation:
```
Changing user to drand
2024-06-28T08:19:55.671Z	INFO	key/store.go:222	Detected stores	{"folder": "/data/drand/.drand/multibeacon", "amount": 1}
2024-06-28T08:19:55.678Z	WARN	key/store.go:234	could not load group, please report this unless this is a new node	{"beaconID": "default", "err": "open /data/drand/.drand/multibeacon/default/groups/drand_group.toml: no such file or directory"}
drand 2.0.1 (date 24/06/2024@14:21:38, commit )
2024-06-28T08:19:55.695Z	INFO	0.0.0.0:8080	core/drand_daemon.go:190	DrandDaemon initialized	{"private_listen": "0.0.0.0:8080", "control_port": "8888", "folder": "/data/drand/.drand/multibeacon", "storage_engine": "bolt"}
2024-06-28T08:19:55.701Z	INFO	0.0.0.0:8080	core/drand_daemon.go:385	beacon id [default]: will run as fresh install -> expect to run DKG.
```

If we try and run `curl -v 127.0.0.1:8080/chains` we won't get anything back! The private listening port only speaks gRPC and is used when
nodes talk to one another. To expose the randomness itself, we must provide a public listening port.

Kill the container and rerun it with a command such as:
```shell
docker run --rm -d -p"8080:8080" -p"8888:8888" -p"9080:9080" --name drand  --volume drand:/data/drand ghcr.io/drand/go-drand:latest start --private-listen 0.0.0.0:8080 --public-listen 0.0.0.0:9080
```

Now if we run `curl -v 127.0.0.1:9080/chains` we should get a 200 response back and an empty list of chains.

## Running with docker compose
This dir contains a sample [docker-compose.yml](./docker-compose.yml) that can be used to spin up a single node. You will still have to go through
the steps of creating a volume and keypair above to use it, though the volume names may vary.

Additionally, you can easily set up a test network of three nodes by running [the start-network.sh script](./start-network.sh).
It can be torn down and cleaned up by using the [./cleanup.sh shell script](./cleanup.sh).
This manifest will spin up a network of three nodes and run an initial distributed key generation process, and they will start generating randomness beacons.

## Running a local test network

For local development and testing, you can use the [start-local-network.sh](./start-local-network.sh) script along with [docker-compose-local-network.yml](./docker-compose-local-network.yml) to run a local network of 3 nodes.

> [!IMPORTANT]
> The docker-compose-local-network.yml builds from source rather than using the prebuilt `ghcr.io/drand/go-drand-local` image. This is because the prebuilt image doesn't properly support the `conn_insecure` build tag needed for local development with multiple nodes communicating with each other. Instead, the compose file passes `buildTag: conn_insecure` as a build argument to ensure this flag is included.

This setup differs from the standard network script by:

1. Building with the `conn_insecure` tag to allow local node-to-node communication without TLS
2. Setting up a more complete DKG configuration with additional parameters:
   - Explicitly specifying all parameters for `generate-proposal` to ensure correct node addressing
   - Using an increased `genesis-delay` (60s instead of the default) to provide enough time for network initialization
   - Setting additional parameters like `catchup-period` and `timeout` for better local testing
3. Including a delay to ensure followers have time to join before executing the DKG

To run the local test network:

```shell
./start-local-network.sh
```

This will create a three-node network and automatically perform the distributed key generation. Once complete, you can access the public API endpoints at:
- Node 1: http://127.0.0.1:9010
- Node 2: http://127.0.0.1:9020
- Node 3: http://127.0.0.1:9030

For more details about drand commands and DKG operations, see the [drand CLI documentation](https://docs.drand.love/operator/drand-cli/#drand-dkg).

## Running with nginx
Many LoE partners like to run a reverse proxy in front of their node to easily manage TLS termination, domain names and firewalling.
In [docker-compose-nginx.yml](./docker-compose-nginx.yml) you can find a manifest for running a single drand docker container and an
nginx container to route traffic to it. Similar to the keypair, we will have to create a volume containing the nginx config (and any TLS config you wish to add).
