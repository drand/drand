[![Build Status](https://travis-ci.org/dedis/drand.svg?branch=master)](https://travis-ci.org/dedis/drand)

# Drand - A Distributed Randomness Beacon Daemon

Drand (pronounced "dee-rand") is a distributed randomness beacon daemon written
in [Golang](https://golang.org/). Servers running drand can be linked with each
other to produce collective, publicly verifiable, unbiasable, unpredictable
random values at fixed intervals using bilinear pairings and threshold cryptography.
Drand nodes can also serve locally-generated private randomness to clients.

### Disclaimer

**This software is considered experimental and has NOT received a third-party
audit yet. Therefore, DO NOT USE it in production or for anything security
critical at this point.**

## Quickstart Tutorial

To run drand locally make sure that you have a working 
[Docker installation](https://docs.docker.com/engine/installation/). 
Then execute:

```bash
git clone https://github.com/dedis/drand
cd drand
./run_local.sh
```

The script spins up six local drand nodes using Docker and produces fresh
randomness every few seconds. 

## Drand Overview

### Public Randomness

Generating public randomness is the primary functionality of drand. Public
randomness is secret only up to a certain point in time after which it is
publicly released. The challenge here is that no party involved in the
randomness generation process should be able to predict or bias the final
output. Additionally, the final result has to be third-party verifiable to make
it actually useful for applications like lotteries, sharding, or parameter
generation in security protocols.

A drand randomness beacon is composed of a distributed set of nodes and has two
phases:

- **Setup:** Each node first generates a *long-term public/private key pair*.
Then all of the public keys are written to a *group file* together with some
further metadata required to operate the beacon. After this group file has
been distributed, the nodes perform a *distributed key generation* (DKG) protocol
to create the collective public key and one private key share per server. The
participants NEVER see/use the actual (distributed) private key explicitly but
instead utilize their respective private key shares for the generation of public
randomness.
- **Generation:** After the setup, the nodes switch to the randomness
generation mode. Any of the nodes can initiate a randomness generation round
by broadcasting a message which all the other participants sign using a t-of-n
threshold version of the *Boneh-Lynn-Shacham* (BLS) signature scheme and their
respective private key shares. Once any node (or third-party observer) has
gathered t partial signatures, it can reconstruct the full BLS
signature (using Lagrange interpolation) which corresponds to the collective
random value. This random beacon / full BLS signature can be verified against
the collective public key.

### Private Randomness

Private randomness generation is the secondary functionality of drand. Clients
can request private randomness from some or all of the drand nodes which extract
it locally from their entropy pools and send it back in encrypted form. This
might be useful to gather randomness from different entropy sources, for example
in embedded devices.

In this mode we assume that a client has a private/public key pair and
encapsulates its public key towards the server's public key using the ECIES
encryption scheme. After receiving a request, the drand node produces 32 random
bytes locally (using Go's `crypto/rand` interface), encrypts them using the
received public key and sends it back to the client.

**Note:** Assuming that clients without good local entropy sources (such
as embedded devices) use this process to gather high entropy randomness to
bootstrap their local PRNGs, we emphasize that the initial client key pair has
to be provided by a trusted source (such as the device manufacturer). Otherwise
we run into the chicken-and-egg problem of how to produce on the client's side a
secure ephemeral key pair for ECIES encryption without a good (local) source of
randomness.

## Installation

Drand can be installed via [Golang](https://golang.org/) or
[Docker](https://www.docker.com/). By default, drand saves the configuration
files such as the long-term key pair, the group file, and the collective public
key in the directory `$HOME/.drand/`.

### Via Docker

Make sure that you have a working [Docker installation](https://docs.docker.com/engine/installation/).

### Via Golang

Make sure that you have a working [Golang installation](https://golang.org/doc/install) and that your [GOPATH](https://golang.org/doc/code.html#GOPATH) is set.
Then install drand via:

```bash
go get -u github.com/dedis/drand
```

## Usage

### TLS

Drand nodes attempt to communicate by default over TLS-protected connections.
Therefore, you need to point your node to the TLS certificate chain and
corresponding private key you wish to use via:

```bash
drand <command> \
    --tls-cert <fullchain.pem> \
    --tls-key <privkey.pem>
```

To get TLS certificates for free you can use, for example, [Let's
Encrypt](https://letsencrypt.org/) and  [EFF's certbot](https://certbot.eff.org/).

Although we **do not recommend** it, you can always disable TLS in drand via:

```bash
drand <command> --insecure
```

### Docker

If you run drand in Docker, **always** use the following template

```bash
docker run \
    --rm \
    --name drand \
    -p <port>:<port> \
    --volume $HOME/.drand/:/root/.drand/ \
    dedis/drand <command>
```

where `<port>` specifies the port through which your drand daemon is reachable
and `<command>` has to be substituted by one of the respective drand
commands below. You must add the corresponding volumes pointing to your TLS
private key and certificate in case you are using TLS (recommended).

### Setup

The setup process for a drand beacon consists of three steps:

1. Generate the long-term key pair for each node
2. Setup the group configuration file
3. Run the distributed key generation (DKG)

#### Long-Term Key Pair Generation

To generate the long-term key pair `drand_id.{secret,public}`, execute

```bash
drand keygen <address>
```

where `<address>` is the address from which your drand daemon is reachable. If
you specify the (no-) TLS flags, as mentioned above, the file `drand_id.public`
will indicate that your node is (not) reachable through TLS.

#### Group Configuration

To generate the group configuration file `group.toml`, run

```bash
drand group <pk1> <pk2> ... <pkn>
```

where `<pki>` is the public key file `drand_id.public` of the i-th participant.
This group file **MUST** be distributed to all participants.

#### Distributed Key Generation

After receiving the `group.toml` file, participants can start drand via:

```bash
drand run \
    --tls-cert <fullchain.pem> \
    --tls-key <privkey.pem> \
    --group-init <group.toml>
```

One of the nodes has to function as the leader which finalizes the setup and
later also initiates the randomness generation rounds. To start the drand
daemon in leader mode, execute:

```bash
drand run \
    --leader \
    --tls-cert <fullchain.pem> \
    --tls-key <privkey.pem> \
    --group-init <group.toml>
```

Once running, the leader initiates the distributed key generation protocol
where all nodes in `group.toml` collectively compute the distributed public key
`dist_key.public` and their respective private key shares `dist_key.private`.

Once the DKG has finished, the keys are stored as
`$HOME/.drand/groups/dist_key.{public,private}`. The distributed public key is
additionally save in the currently active directory.

### Randomness Generation

After a successful setup, drand switches to the randomness generation
mode, where each node broadcasts randomness shares in regular intervals.
Once a node has collected a threshold of shares in the current phase, it
re-creates the public random value and stores it in its local instance of
[BoltDB](https://github.com/coreos/bbolt).

To change the default [interval length](https://golang.org/pkg/time/#ParseDuration) 
from 1 minute to 30 seconds, for example, start drand via

```bash
drand run \
    --leader \
    --period 30s \
    --tls-cert <fullchain.pem> \
    --tls-key <privkey.pem> \
```

**Note:** If a group file is provided at this point, the existing beacon
database will be erased.

### Public Services

A drand beacon provides several public services to clients. Communication is
protected through TLS by default. If the contacted node is using a self-signed
certificate, the client can use the `--tls-cert` flag to specify the server's
certificate.

#### Fetching the Distributed Public Key 

To retrieve the distributed public key of a drand beacon, execute

```bash
drand fetch dist_key <address>
```

where `<address>` is the address of a drand node.

#### Fetching Public Randomness

To get the latest public random value, run

```bash
drand fetch public --distkey <dist_key.public> <address>
```

where `<dist_key.public>` is the distributed public key generated during the setup
and `<address>` is the address of the drand node to be contacted.

The JSON-formatted output produced by drand is of the following form:

```json
{
    "idx": 2,
    "prv": "jnbvQ3LSxg8kp8qwpPO1u2F4ietJCZmMjJUQ1KDo4u94P57hN1K2mJk7oeiWU2Czb5pNqWy4u6vQH2fkqdoNgA==",
    "rnd": "QqcL2Pncns2pKrSdfJw0RK6YFosLpP/44FUBF6Udf38uz5rHyVsZ8/XgElTdDLCpUDIm/DWIzltIzmqArZTjlQ=="
}
```

Here `rnd` is the latest random value, which is a threshold BLS signature on the
previous random value `prv`. The field `idx` specifies the index of `rnd` in the
sequence of all random values produced by this drand instance. Both `rnd` and
`prv` are encoded in base64.

#### Fetching Private Randomness

To get a private random value, run

```bash
drand fetch private <server-identity.toml>
```

where `<server-identity.toml>` is the public identity file of a drand node.

The JSON-formatted output produced by drand is of the following form:

```json
{
    "rnd": "QvIntnAk9P+B3fVQXm3wahNCusx2fKQs0HMRHI77XRk="
}
```

Here `rnd` is the 32-byte base64-encoded private random value produced by the
contacted drand node. If the encryption is not correct, the command outputs an
error instead.

### Private Control Port

Drand provides an interface through which an administrator can interact with a
running node, e.g., to update group details or retrieve secret information. This
control port is only accessible via localhost.

#### Retrieve Private Share

To retrieve the private (DKG) key share run the following command:

```bash
drand control share
```

The JSON-formatted output has the following form:

```json
{
  "index" : 1,
  "share" : {
    "gid": 22,
    "data": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAE="
  }
}
```

## Cryptography used in Drand

Drand relies on the following cryptographic constructions:

- [Pairing-based cryptography](https://en.wikipedia.org/wiki/Pairing-based_cryptography) and [Barreto-Naehrig curves](https://github.com/dfinity/bn).
- [Pedersen's distributed key generation protocol](https://link.springer.com/article/10.1007/s00145-006-0347-3) for the setup.
- Threshold [BLS signatures](https://www.iacr.org/archive/asiacrypt2001/22480516.pdf) for the generation of public randomness.
- [ECIES](https://en.wikipedia.org/wiki/Integrated_Encryption_Scheme) for the encryption of private randomness.
 
For a more general overview on generation of public randomness, see the paper 
[Scalable Bias-Resistant Distributed Randomness](https://eprint.iacr.org/2016/1067.pdf).

## What's Next?

Although being already functional, drand is still at an early development stage
and there is a lot left to be done. Feel free to submit feature requests or,
even better, pull requests. ;)

+ Support DKG timeouts
+ Add more unit tests
+ Reduce size of Docker
+ Add a systemd unit file
+ Support multiple drand instances within one node
+ Implement a more [failure-resilient DKG protocol](https://eprint.iacr.org/2012/377.pdf)

## License

The drand source code is released under MIT license, see the file
[LICENSE](https://github.com/dedis/drand/blob/master/LICENSE) for the full text.

## Designers and Contributors

- Nicolas Gailly ([@nikkolasg1](https://twitter.com/nikkolasg1))
- Philipp Jovanovic ([@daeinar](https://twitter.com/daeinar))
- Mathilde Raynal

## Acknowledgments

Thanks to [@herumi](https://github.com/herumi) for providing support for his
optimized pairing-based cryptographic library used in the first version.

Thanks to Apostol Vassilev for its interest in drand and the long emails
exchanged over the general drand design.

Thanks to [@Bren2010](https://github.com/Bren2010) and
[@grittygrease](https://github.com/grittygrease) for providing the native Golang
bn256 implementation and for their help in the design of drand.
