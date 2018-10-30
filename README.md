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
## Components

drand has two components:

+ a *daemon* that runs the DKG protocol and the randomness beacon
+ a local *control client* that issues command to the daemon so start the DKG
  protocol, or retrieve informations.

The daemon only listens for *localhost* connection from the control client,i.e.
only server's administrators are able to issue commands to their respective drand nodes.

## Usage

This section explains in details the workflow to have a working group of drand
nodes generate randomness. On a high-level, the workflow looks like this:
+ Generate individual longterm key-pair of drand nodes and then the group file that contains all public keys and other informations
+ Start each drand daemons
+ Instruct to each daemons to start the DKG protocol

The randomness beacon automatically starts as soon as the DKG protocol is
finished.

### Setup

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

All informations regarding a group of drand nodes necessary for drand to function properly are located inside a group.toml configuration file. To run a DKG protocol, one needs to generate this group configuration file from all individual longterm keys generated in the previous step. One can do so with:
```
drand group <pk1> <pk2> ... <pkn>
```
where `<pki>` is the public key file `drand_id.public` of the i-th participant.
The group file is generated in the current directory under `group.toml`.
**NOTE:** At this stage, this group file MUST be distributed to all participants !

##### Randomness Beacon Period

drand updates the configuration file after the DKG protocol finishes,
with the distributed public key and automatically starts running the randomness beacon. By default, a randomness beacon has a period of 1mn,i.e. new randomness is generated every minute. If you wish to change the period, you must include that information **inside** the group configuration file. You can do by appending a flag to the command such as :
```
drand group --period 2m <pk1> <pk2> ... <pkn>
```
The period must be parsable by the [time](https://golang.org/pkg/time/#ParseDuration) package.

### Starting drand daemon

There are two ways to run a drand daemon: using TLS or using plain old regular
un-encrypted connections. Drand by default tries to use TLS connections.
The daemon does not go automatically in background, so you must run it with ` &
` in your terminal, or within a screen / tmux session, or with the `-d` option enabled for the docker commands.

#### With TLS

Drand supports by default TLS-encrypted communications. In order to do run drand
in this mode, you need to give at least two options for most operations:
+ `--tls-cert` must point to a valid PEM encoded certificate
+ `--tls-key` must point to a valid TLS private key

These options must be appended to any operations connecting on the network:
`run`, `run dkg` and `run beacon`.

An easy and free way to get TLS certificates these days is to use the [Let's
Encrypt](https://letsencrypt.org/) service, with the official [EFF
tool](https://certbot.eff.org/).

#### Without TLS

Drand is able to run without TLS, mostly intended for testing purpose or for running drand inside a closed network. To run drand without TLS, you need to explicitly tell drand to do so with the `--insecure` flag:

+ `drand keygen --insecure`
+ `drand run --insecure`

#### With Docker

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

**NOTE**: You can test if your daemon is up and running with a ping command on
the same host as the daemon:
```
drand control ping
```

### Distributed Key Generation

After running all drand daemons, each operator needs to issue a command to start
the DKG, using the group file generated before. One can do so with:
```
drand share <group-file>
```

One of the nodes has to function as the leader to initiate the DKG protocol (no
additional trust assumptions), he can do so with:
```
drand share --leader <group-file>
```

Once running, the leader initiates the distributed key generation protocol to
compute the distributed public key (`dist_key.public`) and the private key
shares (`dist_key.private`) together with the participants specified in
`drand_group.toml`.

**Group File**: Once the DKG phase is done, the group file is updated with the
newly created distributed public key. That updated group file needed by drand to
securely contact drand nodes on their public interface to gather private or
public randomness. One can get it via the following: 
```bash
drand show group
``` 
It will print the group file in its regular TOML format. If you want to save it to
a file, append the `--out <file>` flag.

**Distributed Public Key**: More generally, for third party implementation of
randomness beacon verification, one only needs the distributed public key. If
you are an administrator of a drand node, you can use the control port as the
following:
```bash
drand show cokey
```

Otherwise, you can contact an external drand node to ask him for the distributed
public key:
```
drand get cokey <group.toml>
```
The group toml do not need to be updated with the collective key.
**NOTE**:a drand node *can* lie about the key. That information is usually best
gathered from a trusted drand operator and then statically embedded in any
applications using drand.

By default, drand uses TLS, but if do not want to, you can pass the
`--disable-tls` flag. If the remote node is using a self signed certificate for
example, you can use the `--tls-cert` option to specify the certificate of the
server you wish to contact.

### Randomness Generation

We now assume that the setup is done and we can switch to randomness generation.
Note : if a group file is given at this point, the existing beacon database will be erased.

The leader initiates a new randomness generation round automatically after a
successful DKG, as per the specified time interval (default interval: `1m`) in
the group file. All beacon values are stored using
[`BoltDB`](https://github.com/coreos/bbolt), a Go native fast key/value database
engine.


### Randomness Gathering

+ **Public Randomness**: To get the latest public beacon, run the following:
```bash
drand get public group.toml
```
`group.toml` is the group 
the remote node **over TLS**. If the remote node is not using encrypted
communications, then you can pass the `--insecure` flag. If the remote node is
using a self signed certificate for example, you can use the `--tls-cert` option
to specify the certificate of the server you wish to contact.

The output will have the following JSON format:
```json
{
    "Round": 3,
    "Previous": "12392dd64f6628c791fbde72ee8355cf6bc6500c5ba8fadf13ae7f27c688ecf06
61df2092ba0efa4939ac2d19097202be51a7b452346e24a9a794ed8a905cc41",
    "Randomness": "58e6e7a30648846b52d1a586bf45c6f3dcd1824308613002164bbd2442e1bc5
a75826ab335cbe0d26862d33b7f7b9305076e95a8bb67adc2fd7be643672b4e29"
}
```
The public random value is the field `signature`, which is a valid BLS
signature. The signature is made over the concatenation of the `previous_sig`
and the `timestamp` (uint64) field. If the signature is valid, that guarantees a
threshold of drand nodes computed this signature without being able to bias the
outcome.

+ **Private Randomness**: To get a private random value, run the following:
```bash
drand get private group.toml
```
will output
```bash
{
    "Randomness": "764f6e3eecdc4aba8b2f0119e7b2fd8c35948bf2be3f87ebb5823150c6065764"
}
```
`<server_identity.toml>` is the public identity file of one of the server. It is
useful to be able to encrypt both the request and response between the client
and the server. Drand will contact the node over TLS if the identity file has
the "TLS" variable set to `True`.  If the remote node is using a self signed
certificate for example, you can use the `--tls-cert` option to specify the
custom certificate.


The command outputs a 32-byte hex-encoded random value coming from the local
randomness engine of the contacted server. If the encryption is not correct, the
command outputs an error instead.

#### Using HTTP endpoints

Sometimes you may want get the distributed key or public randomness by issuing a GET to a HTTP endpoint instead of using
a gRPC client.

To curl the distributed key, you can use
```bash
curl <address>/info/dist_key
```

Similarly, to get the latest round of randomness from the drand beacon, you can use
```bash
curl <address>/public
```


### Control Port

Unlike the randomness generation or its output, some actions or data must have restricted access. Thus the control functionalities define a set of administrator-level commands, only accessible from localhost. They allow the owner of a drand node to modify the running drand instance (such as adding a new member to the group, ...) and to access their private information.

#### Private Share

To get your private key share generated during the DKG phase, run the command :
```bash
drand control share
```
The output will have the following JSON format :
```json
{
  "index" : 1,
  "share" : {
    "gid": 22,
    "data": "764f6e3eecdc4aba8b2f0119e7b2fd8c35948bf2be3f87ebb5823150c6065764"
  }
}
```

### Updating a drand group

drand allows for "semi-dynamic" group update with a *resharing* protocol that
can do the following:
+ new nodes can join an existing group and get shares. Note that all nodes get *new* shares after running the resharing protocol.
+ nodes can leave their current group. It may be necessary for nodes that do not
  wish to operate drand anymore or *have been deemed not trustworthy enough*.
+ nodes can update the threshold associated with their current distributed
  public key.

The main advantage of this method is that the distributed public key stays the
*same* even with new nodes coming in. That is useful in the case the distributed
public key is embedded in the application using drand, and hence is difficult to
update.

Updating is simple in drand, it uses the same command as for the DKG:
```bash
drand share --from old-group.toml new-group.toml
```
for new nodes joining the system. The old group toml is fetched as shown above,
and the new group toml is created the usual way (`drand group ....`).
For nodes already in the group, there is no need to specify the old group, since
drand already knows it:
```bash
drand share <newGroup.toml>
```

As usual, a leader must start the protocol by indicating the `--leader` flag.
After the protocol is finished, each node listed in the new-group.toml file,
will have a new share corresponding to the same distributed public
key. The randomness generation starts immediately after the resharing protocol

## Learn More About The Crypto Magic Behind Drand

You can learn more about drand, its motivations and how does it work on these
public [slides](https://docs.google.com/presentation/d/1t2ysit78w0lsySwVbQOyWcSDnYxdOBPzY7K2P9UE1Ac/edit?usp=sharing).

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
