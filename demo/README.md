# Local demo of drand

This folder contains code that spins up drand nodes in the same way as in a real
world deployment. It uses real processes as drand instances and uses the CLI
commands.

## What the demo is doing

It prints out much information on its standard output so you can see what are
the steps the demo is performing:
* Setting up a new network from scratch (running the DKG)
* Getting some beacons
* Stopping a node and checking the network is still alive
* Getting node back and checking it has caught up the chain
* Doing a resharing to an extended group
* Checking if the new network produces valid random beacons

## Run the demo

```
go build && ./demo -build
```

You can stop the demo by CTRL-C whenever you want.

### Running the demo with PostgreSQL

Drand supports PostgreSQL as a database backend for beacons.

To run with PostgreSQL support, use the following command:

```shell
go build && ./demo -build -db=postgres
```

## Fetching randomness

You can fetch randomness by running the command written out by the demo.

## Inspecting nodes

All temporary files are written to `/tmp/drand-full`.
You can inspect the private key, share, group and log of all nodes in
`/tmp/drand-full/node-X/`.
