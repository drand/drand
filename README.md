[![Build Status](https://travis-ci.org/dedis/drand.svg?branch=master)](https://travis-ci.org/dedis/drand)

# Drand - A Distributed Randomness Beacon Daemon

Drand (pronounced "dee-rand") is a distributed randomness beacon daemon written
in [Golang](https://golang.org/). Servers running drand can be linked with each
other to produce collective, publicly verifiable, unbiasable, unpredictable
random values at fixed intervals using pairing-based threshold cryptography.
Nodes running drand can also serve individual requests to serve locally-generated
private randomness to a client.

### Disclaimer

**This software is considered experimental and has NOT received a
full audit yet. Therefore, DO NOT USE it in production at this point. You have
been warned.**

## I Want Randomness Now!

Sure thing, here you go:

1. Make sure that you have a working [Docker installation](https://docs.docker.com/engine/installation/).
2. Then run:
```bash
./run_local.sh
```

The script spins up six local drand nodes and produces fresh randomness every two
seconds. Drand is able to produce two kind of randomness:
+ Drand main's function is to generate verifiable unbiasable randomness that we
  call **Public Randomness**. This kind of randomness is useful in many
  applications such as lottery, or sharding or even parameters generation.
+ Drand can also generate **Private Randomness**. This randomness has been
  generated locally by the remote server who sends it back in an encrypted form
  to the client. This is useful to gather different entropy sources to generate
  a high entropy randomness source.

## Drand in a Nutshell

### Public Randomness

A drand distributed randomness beacon involves a set of nodes and has two phases:

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

### Private Randomness

The private randomness functionality can be used when one needs some high entropy randomness.
A client can contact many different drand nodes individually and retrieve a portion of their
local randomness. This randomness can later be used to generate private key, nonces, etc. It
is particularly useful when the local randomness generator lacks external entropy, for example
in embedded devices.

In this mode, the client generates an ephemereal private/public key pair and encrypts the
public key towards the server's public key using the ECIES encryption scheme.
Upon reception of the request, the server produces 32 random bytes locally
(using Go's `crypto/rand` interface), and encrypts back the randomness to the
client's public key. Of course, this is only a first version and much more thinking must be put
into the chicken-and-egg problem: how to generate an ephemereal key pair to get randomness
if we have bad randomness in the first place. We can later assume that the device is given an
initial key pair which uses it to gather randomness from drand nodes. This is not yet formally
decided nor implemented yet and any comments/ideas on this are most welcomed.

## Installation

Drand can be installed via [Golang](https://golang.org/) or [Docker](https://www.docker.com/).
By default, drand saves the configuration files such as the long-term key pair, the group file,
and the collective public key in `$HOME/.drand/`.

### Via Docker

Make sure that you have a working [Docker installation](https://docs.docker.com/engine/installation/).

### Via Golang

1. Make sure that you have a working [Golang installation](https://golang.org/doc/install) and that your [GOPATH](https://golang.org/doc/code.html#GOPATH) is set.
3. Install drand via:
```
go get -u github.com/dedis/drand
```

## Usage

There are two ways to run a drand node: using TLS or using plain old regular
un-encrypted connections. Drand by default tries to use TLS connections.

### With TLS

Drand supports by default TLS-encrypted communications. In order to do run drand
in this mode, you need to give at least two options for most operations:
+ `--tls-cert` must point to a valid PEM encoded certificate
+ `--tls-key` must point to a valid TLS private key

These options must be appended to any operations connecting on the network:
`run`, `run dkg` and `run beacon`.

An easy and free way to get TLS certificates these days is to use the [Let's
Encrypt](https://letsencrypt.org/) service, with the official [EFF
tool](https://certbot.eff.org/).

### Without TLS

Drand is able to run without TLS, mostly intended for testing purpose or for running drand inside a closed network. To run drand without TLS, you need to explicitly tell drand to do so with the `--insecure` flag:

+ `drand keygen --insecure`
+ `drand run --insecure`

### With Docker

**NOTE:** If you run drand in Docker, always use the following template
```
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
private key and certificate in case you are using TLS, which you should.

## Setup

To setup the drand beacon, each participant generates its long-term key pair
from which we can then assemble the group configuration file, and finally all
participants run the distributed key generation protocol.

#### Long-Term Key

To generate the long-term key pair `drand_id.{secret,public}` of the drand daemon, execute
```
drand keygen <address>
```
where `<address>` is the address from which your drand daemon is reachable. The
address must be reachable over a TLS connection. In case you need non-secured
channel, you can pass the `--insecure` flag.

#### Group Configuration

To generate the group configuration file `drand_group.toml`, run
```
drand group <pk1> <pk2> ... <pkn>
```
where `<pki>` is the public key file `drand_id.public` of the i-th participant.
The group file is generated in the current directory under `group.toml`.

**NOTE:** This group file MUST be distributed to all participants !

#### Distributed Key Generation

After receiving the `drand_group.toml` file, participants can start drand via:
```
drand run --tls-cert <cert path> --tls-key <key path> --group-init <group_file.toml>
```
where `<cert path>` is the path of your TLS certificate and `<key path>` is the
path of your TLS private key. If you need non-secured channel, you can use the
`--insecure` option.

One of the nodes has to function as the leader which finalizes the setup and
later also initiates regular randomness generation rounds. To start the drand
daemon in leader mode, execute:
```
drand run --leader --tls-cert <cert path> --tls-key <key path> --group-init <group_file.toml>
```

Once running, the leader initiates the distributed key generation protocol to
compute the distributed public key (`dist_key.public`) and the private key
shares (`dist_key.private`) together with the participants specified in
`drand_group.toml`.

Once the DKG phase is done, the distributed public key is saved in the local directoryas well as in the configuration folder (`$HOME/.drand` by default) under the file `groups/dist_key.public`.

#### Downloading the distributed public key

To get the distributed public key of a drand protocol run, execute :
```bash
drand fetch dist_key <address>
```
`<address>` being the address the drand node to contact.
Please note that a drand node can currently only being part of one "drand network" and thus handle only one distributed key. To get several networks, different instances of drand need to be ran.
By default, drand uses TLS, but if do not want to, you can pass the `--insecure` flag. If the remote node is
using a self signed certificate for example, you can use the `--tls-cert` option
to specify the certificate of the server you wish to contact.

### Randomness Generation

We now assume that the setup is done and we can switch to randomness generation.
Note : if a group file is given at this point, the existing beacon database will be erased.

The leader initiates a new randomness generation round automatically as per the
specified time interval (default interval: `1m`). All beacon values are stored
using [`BoltDB`](https://github.com/coreos/bbolt), a Go native fast key/value
database engine.

To change the [duration](https://golang.org/pkg/time/#ParseDuration) of the
randomness generation interval, e.g., to `30s`, start drand via
```
drand run --leader --period 30s --tls-cert <cert path> --tls-key <key path>
```

### Randomness Gathering

+ **Public Randomness**: To get the latest public beacon, run the following:
```bash
drand fetch public --distkey dist_key.public <address>
```
`dist_key.public` is the distributed key generated once the DKG phase completed,
and `<address>` is the address of one drand node. By default, drand contacts
the remote node **over TLS**. If the remote node is not using encrypted
communications, then you can pass the `--insecure` flag. If the remote node is
using a self signed certificate for example, you can use the `--tls-cert` option
to specify the certificate of the server you wish to contact.

The output will have the following JSON format:
```json
{
    "round": 2,
    "previous_rand": "jnbvQ3LSxg8kp8qwpPO1u2F4ietJCZmMjJUQ1KDo4u94P57hN1K2mJk7oeiWU2Czb5pNqWy4u6vQH2fkqdoNgA==",
    "randomness": "QqcL2Pncns2pKrSdfJw0RK6YFosLpP/44FUBF6Udf38uz5rHyVsZ8/XgElTdDLCpUDIm/DWIzltIzmqArZTjlQ=="
}

```
The public random value is the field `signature`, which is a valid BLS
signature. The signature is made over the concatenation of the `previous_sig`
and the `timestamp` (uint64) field. If the signature is valid, that guarantees a
threshold of drand nodes computed this signature without being able to bias the
outcome.

+ **Private Randomness**: To get a private random value, run the following:
```bash
drand fetch private <server_identity.toml>
```
will output
```bash
{
    "randomness": "QvIntnAk9P+B3fVQXm3wahNCusx2fKQs0HMRHI77XRk="
}
```
`<server_identity.toml>` is the public identity file of one of the server. It is
useful to be able to encrypt both the request and response between the client
and the server. Drand will contact the node over TLS if the identity file has
the "TLS" variable set to `True`.  If the remote node is using a self signed
certificate for example, you can use the `--tls-cert` option to specify the
custom certificate.


The command outputs a 32-byte base64-encoded random value coming from the local
randomness engine of the contacted server. If the encryption is not correct, the
command outputs an error instead.

### Control

Unlike the randomness generation or its output, some actions or data must have restricted access. Thus the control functionalities define a set of commands that communicate on localhost to allow privacy and security. They allow the owner of a drand node to modify the running drand instance and to access their private information.

#### Share

To get your private key share generated during the DKG phase, run the command :
```bash
drand control share --insecure
```

## Learn More About The Crypto Magic Behind Drand

Drand relies on the following cryptographic constructions:
- All drand protocols rely on [pairing-based cryptography](https://en.wikipedia.org/wiki/Pairing-based_cryptography) using
  an optimized implementation of the [Barreto-Naehrig curves](https://github.com/dfinity/bn).
- For the setup of the distributed key, drand uses an implementation of
  [Pedersen's distributed key generation protocol](https://link.springer.com/article/10.1007/s00145-006-0347-3).
  There are more [advanced DKG protocols](https://eprint.iacr.org/2012/377.pdf) which we plan to implement in the future.
- For the randomness generation, drand uses an implementation of threshold
  [BLS signatures](https://www.iacr.org/archive/asiacrypt2001/22480516.pdf).
- For the encryption used in the private randomness gathering, see the [ECIES
  scheme](https://en.wikipedia.org/wiki/Integrated_Encryption_Scheme).
- For a more general overview on generation of public randomness, see the
  paper [Scalable Bias-Resistant Distributed Randomness](https://eprint.iacr.org/2016/1067.pdf)

## What's Next?

Although being already functional, drand is still at an early stage of
development, so there's a lot left to be done. Feel free to submit feature or,
even better, pull requests. ;)

+ dkg timeout
+ interoperable different groups
+ more unit tests
+ reduce Docker size by building first and copy in fresh container
+ systemd unit file

## License
The drand source code is released under MIT license, see the file
[LICENSE](https://github.com/dedis/drand/blob/master/LICENSE) for the full text.

## Designers and Contributors

- Nicolas Gailly ([@nikkolasg1](https://twitter.com/nikkolasg1))
- Philipp Jovanovic ([@daeinar](https://twitter.com/daeinar))

## Acknowledgments

Thanks to [@herumi](https://github.com/herumi) for providing support for his
optimized pairing-based cryptographic library used in the first version.

Thanks to Apostol Vassilev for its interest in drand and the long emails
exchanged over the general drand design.

Thanks to [@Bren2010](https://github.com/Bren2010) and
[@grittygrease](https://github.com/grittygrease) for providing the native Golang
bn256 implementation and for their help in the design of drand.
