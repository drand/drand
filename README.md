[![Build Status](https://travis-ci.org/dedis/drand.svg?branch=master)](https://travis-ci.org/dedis/drand)

# Drand - A Distributed Randomness Beacon Daemon

Drand (pronounced "dee-rand") is a distributed randomness beacon daemon written
in [Golang](https://golang.org/). Servers running drand can be linked with each
other to produce collective, publicly verifiable, unbiasable, unpredictable
random values at fixed intervals using pairing-based threshold cryptography.

### Disclaimer

**This software is considered experimental and has NOT received a
full audit yet. Therefore, DO NOT USE it in production at this point. You have
been warned.**

## Drand in a Nutshell

A drand distributed randomness beacon is created with a set of nodes and has two phases:

- **Setup:** Each node first generates a *long-term public/private key
    pair*. Afterwards, a *group file* is created which gathers all the
    participants' public keys together with some further metadata required to
    operate the beacon. After the group file has been distributed, all
    participants run a *distributed key generation* (DKG) protocol to create
    the collective public key and one private key share per node. The
    participants NEVER see/use the actual private key explicitly but instead
    utilize their respective private key shares for drand's cryptographic
    operations.

- **Generation:** After the setup, the participating nodes switch to the
    randomness generation mode. Any of the nodes can then function as a leader
    to initiate a randomness generation round. Therefore, a given leader broadcasts
    a message (in this case, a timestamp) which is then signed by all
    participants using a threshold version of the *Boneh-Lynn-Shacham* (BLS)
    signature scheme and their respective private key shares. Once any node (or
    third-party observer) has gathered a threshold of partial signatures, it can
    reconstruct the full BLS signature (using Lagrange interpolation) which
    corresponds to the collective random value. This random beacon / full BLS
    signature can be verified against the distributed public key that was
    computed with the DKG.

## Installation 

Drand can be installed via [Golang](https://golang.org/) or [Docker](https://www.docker.com/). 
As a first step create drand's application folder where the configuration files
such as the long-term key pair, the group file, and the collective public key
are stored:
```
mkdir $HOME/.drand/
```

### Via Docker

1. Make sure that you have a working [Docker installation](https://docs.docker.com/engine/installation/). 
2. Pull the latest [drand image](https://hub.docker.com/r/dedis/drand/) from Docker Hub: 
```
docker pull dedis/drand
```

### Via Golang

1. Make sure that you have a working [Golang installation](https://golang.org/doc/install) and that your [GOPATH](https://golang.org/doc/code.html#GOPATH) is set.
2. Install the [pairing-based crypto library](https://github.com/dfinity/bn). **TODO**
3. Install drand via:
```
go get github.com/dedis/drand
```

## Usage

**NOTE:** If you run drand in Docker, always use the following template
```
docker run \ 
    --rm \ 
    --name drand 
    --port <port>:<port> \ 
    --volume $HOME/.drand/:/root/.drand/ \ 
    dedis/<command>
```
where `<port>` specifies the port through which your drand daemon is reachable
and `<command>` has to be substituted by one of the respective drand
commands below.

### Setup

First we need to setup the drand daemon by generating its long-term key pair,
assemble the group configuration file, and run the distributed key generation
protocol.

#### Long-Term Key

To generate the long-term key pair `drand_id.{secret,public}` of the drand daemon, execute
```
drand keygen <ip>:<port>
```
where `<ip>:<port>` is the address from which your drand daemon is reachable.

**NOTE:** If you use Docker, make sure to use the same `<port>` value consistently.

#### Group Configuration

To generate the group configuration file `drand_group.toml`, run
```
drand group <pk1> <pk2> ... <pkn>
```
where `<pki>` is the public key file `drand_id.public` of the i-th participant.

**NOTE:** This group file MUST be distributed to all participants and MUST be
stored in the respective application folder (e.g., `$HOME/.drand`).

#### Distributed Key Generation

After receiving the `drand_group.toml` file, participants can start drand via:
```
drand run
```

One of the nodes has to function as the leader which finalizes the setup and
later also initiates regular randomness generation rounds.
To start the drand daemon in leader mode, execute:
```
drand run --leader
```

Once running, the leader initiates (as the final part of the setup) the
distributed key generation protocol to compute the distributed public key
(`dist_key.public`) and the private key shares (`dist_key.private`) together
with the participants specified in `drand_group.toml`.

### Randomness Generation

The leader initiates a new randomness generation round automatically as per the
specified time interval (default interval: `1m`). All beacon values are stored
as `$HOME/.drand/beacons/<timestamp>.sig`.

To change the [duration](https://golang.org/pkg/time/#ParseDuration) of the
randomness generation interval, e.g., to `30s`, start drand via
```
drand run --leader --period 30s
```

### Randomness Verification

To verify a beacon `<timestamp>.sig` using `dist_key.public` simply run:
```
drand verify --distkey dist_key.public <timestamp>.sig
```
The command returns 0 if the signature is valid and 1 otherwise.

## How Can I Learn More About The Crypto Magic Behind Drand?

Drand relies on the following cryptographic constructions:
- All drand protocols rely on [pairing-based cryptography](https://en.wikipedia.org/wiki/Pairing-based_cryptography) using
  an optimized implementation of the [Barreto-Naehrig curves](https://github.com/dfinity/bn).
- For the setup of the distributed key, drand uses an implementation of
  [Pedersen's distributed key generation protocol](https://link.springer.com/article/10.1007/s00145-006-0347-3).
  There are more [advanced DKG protocols](https://eprint.iacr.org/2012/377.pdf) which we plan to implement in the future.
- For the randomness generation, drand uses an implementation of threshold 
  [BLS signatures](https://www.iacr.org/archive/asiacrypt2001/22480516.pdf).
- For a more general overview on generation of public randomness, see the
  paper [Scalable Bias-Resistant Distributed Randomness](https://eprint.iacr.org/2016/1067.pdf)

## What's Next?

Although being already functional, drand is still at an early stage of
development, so there's a lot left to be done. Feel free to submit feature or,
even better, pull requests. ;)

For more details on the open issues see our [TODO](https://github.com/dedis/drand/blob/master/TODO.md) list.

## Designers and Contributors

- Nicolas Gailly ([@nikkolasg1](https://twitter.com/nikkolasg1))
- Philipp Jovanovic ([@daeinar](https://twitter.com/daeinar))

## Acknowledgments

Thanks to [@herumi](https://github.com/herumi) for providing support for his
optimized pairing-based cryptographic library.
