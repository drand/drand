[![CircleCI](https://circleci.com/gh/drand/drand.svg?style=svg)](https://circleci.com/gh/drand/drand)

# Drand - A Distributed Randomness Beacon Daemon
Drand (pronounced "dee-rand") is a distributed randomness beacon daemon written
in [Golang](https://golang.org/). Servers running drand can be linked with each
other to produce collective, publicly verifiable, unbiased, unpredictable
random values at fixed intervals using bilinear pairings and threshold
cryptography. Drand nodes can also serve locally-generated private randomness
to clients.

drand was first developed within the [DEDIS organization](
https://github.com/dedis), and as of December 2019,
is now under the drand organization.

### Disclaimer
**This software is considered experimental and has NOT received a third-party
audit yet. Therefore, DO NOT USE it in production or for anything security
critical at this point.**

# Table of Contents

   * [Drand - A Distributed Randomness Beacon Daemon](#drand---a-distributed-randomness-bea
con-daemon)
         * [Disclaimer](#disclaimer)
      * [Table of Contents](#table-of-contents)
      * [Goal and Overview](#goal-and-overview)
         * [Public Randomness](#public-randomness)
         * [Private Randomness](#private-randomness)
      * [Local demo](#local-demo)
      * [Installation](#installation)
         * [Official release](#official-release)
         * [Manual installation](#manual-installation)
            * [Via Golang](#via-golang)
            * [Via Docker](#via-docker)
         * [TLS setup: Nginx with Let's Encrypt](#tls-setup-nginx-with-lets-encrypt)
      * [Usage](#usage)
         * [Setup](#setup)
            * [Long-Term Key](#long-term-key)
            * [Starting drand daemon](#starting-drand-daemon)
               * [With TLS](#with-tls)
               * [Without TLS](#without-tls)
            * [Run the setup phase](#run-the-setup-phase)
         * [Distributed Key Generation](#distributed-key-generation)
         * [Randomness Generation](#randomness-generation)
         * [Control Functionalities](#control-functionalities)
            * [Long-Term Private Key](#long-term-private-key)
            * [Long-Term Public Key](#long-term-public-key)
            * [Private Key Share](#private-key-share)
            * [Distributed Key](#distributed-key)
         * [Using Drand](#using-drand)
         * [Fetching Public Randomness](#fetching-public-randomness)
            * [Fetching Private Randomness](#fetching-private-randomness)
            * [Using HTTP endpoints](#using-http-endpoints)
         * [Updating Drand Group](#updating-drand-group)
      * [Metrics](#metrics)
      * [DrandJS](#drandjs)
      * [Documentation](#documentation)
      * [What's Next?](#whats-next)
      * [License](#license)
      * [Contributors](#contributors)
      * [Acknowledgments](#acknowledgments)
      * [Coverage](#coverage)
      * [Supporting](#supporting)



## Goal and Overview
The need for digital randomness is paramount in multiple digital applications
([e]voting, lottery, cryptographic parameters, embedded devices bootstrapping
randomness, blockchain systems etc) as well in non-digital such as statistical
sampling (used for example to check results of an election), assigning court
cases to random judges, random financial audits, etc.  However, constructing a
secure source of randomness is nothing but easy: there are countless examples
of attacks where the randomness generation was the culprit (static keys,
non-uniform distribution, biased output, etc).  drand aims to fix that gap by
providing a Randomness-as-a-Service network (similar to NTP servers for time,
or Certificate Authority servers for CAs verification), providing continuous
source of randomness which is:

* Decentralized: drand is a software ran by a diverse set of reputable entities
  on the Internet and a threshold of them is needed to generate randomness,
  there is no central point of failure. 
* Publicly verifiable & unbiased: drand periodically delivers publicly
  verifiable and unbiased randomness. Any third party can fetch and verify the
  authenticity of the randomness and by that making sure it hasn't been
  tampered with.
* And "private" as well: drand nodes can also deliver encrypted randomness 
  to be used in a local applications, for example to seed the OS's PRNG.

Drand currently runs a first test network composed by trustworthy organizations
around the globe such as Cloudflare, EPFL, University of Chile and Kudelski
Security.  The main website of the first launch sponsored by Cloudflare is
hosted at the [league of entropy site](https://leagueofentropy.com).
There is an independent drand website (source in `web/`) showing the same
network hosted in one of the participant's server: https://drand.zerobyte.io

### Public Randomness
Generating public randomness is the primary functionality of drand. Public
randomness is generated collectively by drand nodes and publicly available. The
main challenge in generating good randomness is that no party involved in the
randomness generation process should be able to predict or bias the final
output. Additionally, the final result has to be third-party verifiable to make
it actually useful for applications like lotteries, sharding, or parameter
generation in security protocols.

A drand randomness beacon is composed of a distributed set of nodes and has two
phases:

- **Setup:** Each node first generates a *long-term public/private key pair*.
  Then all of the public keys are written to a *group file* together with some
  further metadata required to operate the beacon. After this group file has
  been distributed, the nodes perform a *distributed key generation* (DKG)
  protocol to create the collective public key and one private key share per
  server. The participants NEVER see/use the actual (distributed) private key
  explicitly but instead utilize their respective private key shares for the
  generation of public randomness.
- **Generation:** After the setup, the nodes switch to the randomness
  generation mode. Any of the nodes can initiate a randomness generation round
  by broadcasting a message which all the other participants sign using a
  t-of-n threshold version of the *Boneh-Lynn-Shacham* (BLS) signature scheme
  and their respective private key shares. Once any node (or third-party
  observer) has gathered t partial signatures, it can reconstruct the full BLS
  signature (using Lagrange interpolation). The signature is then hashed using
  SHA-512 to ensure that there is no bias in the byte representation of the
  final output. This hash corresponds to the collective random value and can be
  verified against the collective public key.

### Private Randomness
Private randomness generation is the secondary functionality of drand. Clients
can request private randomness from some or all of the drand nodes which
extract it locally from their entropy pools and send it back in encrypted form.
This can be useful to gather randomness from different entropy sources, for
example in embedded devices.

In this mode we assume that a client has a private/public key pair and
encapsulates its public key towards the server's public key using the ECIES
encryption scheme. After receiving a request, the drand node produces 32 random
bytes locally (using Go's `crypto/rand` interface), encrypts them using the
received public key and sends it back to the client.

**Note:** Assuming that clients without good local entropy sources (such as
embedded devices) use this process to gather high entropy randomness to
bootstrap their local PRNGs, we emphasize that the initial client key pair has
to be provided by a trusted source (such as the device manufacturer). Otherwise
we run into the chicken-and-egg problem of how to produce on the client's side
a secure ephemeral key pair for ECIES encryption without a good (local) source
of randomness.

## Local demo

To run a local demo, you can simply run:
```bash
make demo
```

The script spins up a few drand local processes, performe resharing and other
operations and will continue to print out new randomness every Xs (currently
6s).
For more information, look at the demo [README](https://github.com/drand/drand/tree/master/demo).

## Installation
### Official release
Please go use the latest drand binary in the
[release page](https://github.com/drand/drand/releases).

### Manual installation
Drand can be installed via [Golang](https://golang.org/) or
[Docker](https://www.docker.com/). By default, drand saves the configuration
files such as the long-term key pair, the group file, and the collective public
key in the directory `$HOME/.drand/`.

#### Via Golang
Make sure that you have a working [Golang
installation](https://golang.org/doc/install) and that your
[GOPATH](https://golang.org/doc/code.html#GOPATH) is set.

Then install drand via:
```bash
git clone https://github.com/drand/drand
cd drand
make install
```

#### Via Docker
The setup is explained in
[README\_docker.md](https://github.com/drand/drand/tree/master/docker/README_docker.md).

### TLS setup: Nginx with Let's Encrypt
Running drand behind a reverse proxy is the **default** method of deploying
drand. Such a setup greatly simplify TLS management issues (renewal of
certificates, etc). We provide here the minimum setup using
[Nginx](https://www.nginx.com/) and
[certbot](https://certbot.eff.org/instructions/) - make sure you have both
binaries installed with the latest version; Nginx version must be at least >=
1.13.10 for gRPC compatibility.

+ First, add an entry in the Nginx configuration for drand:
```bash
# /etc/nginx/sites-available/default
server {
  server_name drand.nikkolasg.xyz;
  listen 443 ssl http2;
 
  location / {
    grpc_pass grpc://localhost:8080;
  }
  location /api/ {
    proxy_pass http://localhost:8080;
    proxy_set_header Host $host;
  }
}
```
**Note**: you can change 
1. the port on which you want drand to be accessible by changing the line
   `listen 443 ssl http2` to use any port.
2. the port on which the drand binary will listen locally by changing the line
   `proxy_pass http://localhost:8080; ` and ` grpc_pass grpc://localhost:8080;`
   to use any local port.

+ Run certbot to get a TLS certificate:
```bash
sudo certbot --nginx
```

+ **Running** drand now requires to add the following options:
```bash
drand start --tls-disable --listen 127.0.0.1:8080
```

The `--listen` flag tells drand to listen on the given address instead of the
public address generated during the setup phase (see below).

## Usage

This section explains in details the workflow to have a working group of drand
nodes generate randomness. On a high-level, the workflow looks like this:
+ **Setup**: generation of individual long-term key pair and the group file and
  starting the drand daemon.
+ **Distributed Key Generation**: each drand node collectively participates in
  the DKG.
+ **Randomness Generation**: the randomness beacon automatically starts as soon
  as the DKG protocol is finished.

### Setup

The setup process for a drand node consists of the following steps:
1. Generate the long-term key pair for each node
2. Each node starts their daemon
2. Leader starts the command as a coordinator & every participant connect to the
   coordinator to setup the network

#### Long-Term Key

To generate the long-term key pair `drand_id.{secret,public}` of the drand
daemon, execute
```
drand generate-keypair <address>
```
where `<address>` is the address from which your drand daemon is reachable. The
address must be reachable over a TLS connection directly or via a reverse proxy
setup. In case you need non-secured channel, you can pass the `--tls-disable`
flag.

#### Starting drand daemon

The daemon does not go automatically in background, so you must run it with ` &
` in your terminal, within a screen / tmux session, or with the `-d` option
enabled for the docker commands. Once the daemon is running, the way to issue
commands to the daemon is to use the control functionalities.  The control
client has to run on the same server as the drand daemon, so only drand
administrators can issue command to their drand daemons.

There are two ways to run a drand daemon: using TLS or using plain old regular
unencrypted connections. Drand by default tries to use TLS connections.

##### With TLS
Drand nodes attempt to communicate by default over TLS-protected connections.
Therefore, you need to point your node to the TLS certificate chain and
corresponding private key you wish to use via:
```bash
drand start \
    --tls-cert <fullchain.pem> \
    --tls-key <privkey.pem>
```

To get TLS certificates for free you can use, for example, [Let's
Encrypt](https://letsencrypt.org/) with its official CLI tool [EFF's
certbot](https://certbot.eff.org/).

##### Without TLS

Although we **do not recommend** it, you can always disable TLS in drand via:
```bash
drand start --tls-disable
```

#### Run the setup phase

To setup a new network, drand uses the notion the of a coordinator that collects
the public key of the participants, setups the group configuration once all keys
are received and then start the distributed key generation phase. Once the DKG
phase is performed, the participants can see the list of members in the group
configuration file

**Coordinator**: The designated coordinator node must run the following command
**before** everyone else:
```
drand share --leader --nodes 10 --threshold 6 --secret mysecret --period 30s
```

**Rest of participants**: Once the coordinator has run the previous command, the 
rest of the participants must run the following command:
```
drand share --connect <leaderaddress> --nodes 10 --threshold 6 --secret mysecret
```

The flags usage is as follow:
* `--leader` indicates this node is a coordinator, `
* `--nodes` indicates how many nodes do we expect to form the network
* `--threshold` indicates the threshold the network should use, i.e. how many
  nodes amongst the total needs to be online for the network to be live at any
  point.
* `--period` indicates the period of the randomness beacon to use. It must be
  valid duration as parsed by Golang's `[time.ParseDuration]`(https://golang.org/pkg/time/#ParseDuration)  method.
* `--secret` indicates the secret that the coordinator uses to authentify the
  nodes that wants to participate to the network.
* `--connect` is the `host:port` address of the leader. By default, drand will
  connect to the leader by using tls. If you are not using tls, use the
  `--tls-disable` flag.

**Interactive command**: The command will run as long as the DKG is not finished
yet. You can quit the command, the DKG will proceed but the group file will not
be written down. In that case, once the DKG is done, you get the group file by
running:
```
drand show group --out group.toml
```


**Secret**: For participants to be included in the group, they need to have a
secret string shared by all. This method is offering some basic security
however drand will provide more manual checks later-on and/or different secrets
for each participants. However, since the set of participants is public and consistent 
accross all participants after a setup, nodes can detect if there are some unwanted nodes
after the setup and in that case, setup a new network again.

**Custom entropy source**: By default drand takes its entropy for the setup
phase from the OS's entropy source (`/dev/urandom` on Unix systems). However,
it is possible for a participant to inject their own entropy source into the
creation of their secret. To do so, one must have an executable that produces
random data when called and pass the name of that executable to drand:
```
drand share <regular options> --source <entropy-exec>
```
where `<entropy-exec>` is the path to the executable which produces the user's
random data on STDOUT.  As a precaution, the user's randomness is mixed by
default with `crypto/rand` to create a random stream. In order to introduce
reproducibility, the flag `user-source-only` can be set to impose that only the
user-specified entropy source is used. Its use should be limited to testing.
```
drand share <group-file> --source <entropy-exec> --user-source-only
```

### Distributed Key Generation

Once the DKG phase is done, each node has both a private share and a group file
containing the distributed public key. Using the previous commands shown, the
group file will be written to `group.toml`. That updated group file is needed by
drand to securely contact drand nodes on their public interface to gather
private or public randomness. A drand administrator can get the updated group
file it via the following:
```bash
drand show group
```
It will print the group file in its regular TOML format. If you want to save it
to a file, append the `--out <file>` flag.

**Distributed Public Key**: More generally, for third party implementation of
randomness beacon verification, one only needs the distributed public key. If
you are an administrator of a drand node, you can use the control port as the
following:
```bash
drand show cokey
```

Otherwise, you can contact an external drand node to ask him for its current
distributed public key:
```bash
drand get cokey <address>
```
where `<group.toml>` is the group file identity file of a drand node. You can
use the flag `--nodes <address(es)>` to indicate which node you want to contact
specifically (it is a white space separated list).
Use the`--tls-cert` flag to specify the server's certificate if
needed. The group toml does not need to be updated with the collective key.

**NOTE**: Using the last method (`get cokey`), a drand node *can* lie about the
key if no out-of-band verification is performed. That information is usually
best gathered from a trusted drand operator and then embedded in any
applications using drand.

### Randomness Generation

After a successful setup phase, drand will switch to the randomness generation
mode *at the genesis time* specified in the group file. At that time, each node
broadcasts randomness shares at regular intervals. Once a node has collected a
threshold of shares in the current phase, it computes the public random value
and stores it in its local instance of
[BoltDB](https://github.com/coreos/bbolt).

**Timings of randomness generation**: At each new period, each node will try to
broadcast their partial signatures for the corresponding round and try to generate 
a full randomness from the partial signatures. The corresponding round is the
number of rounds elapsed from the genesis time. That means there is a 1-1
mapping between a given time and a drand round.

**Daemon downtime & Chain Sync**: Due to the threshold nature of drand, a drand
network can support some numbers of nodes offline at any given point. This
number is determined by the threshold: `max_offline = group_len - threshold`.
When a drand node goes back up, it will sync rapidly with the other nodes to
catch up its local chain and participate in the next upcoming drand round.

**Drand network failure**: If for some reason drand goes down for some time and
then backs up, the new randomn beacon will be built over the *last successfully
generated beacon*. For example, if the network goes down at round 10 (i.e. last
beacon generated contained `round: 10`), and back up again at round 20 (i.e.
field `round: 20`), then this new randomness contains the field
`previous_round:10`. 


### Control Functionalities

Drand's local administrator interface provides further functionality, e.g., to
update group details or retrieve secret information. By default, the daemon
listens on `127.0.0.1:8888`, but you can specify another control port when
starting the daemon with:
```bash
drand start --control 1234
```
In that case, you need to specify the control port for each of the following
commands.

#### Long-Term Private Key
To retrieve the long-term private key of our node, run:
```bash
drand show private
```

#### Long-Term Public Key
To retrieve the long-term public key of our node, run:
```bash
drand show public
```

#### Private Key Share
To retrieve the private key share of our node, as determined during the DKG,
run the following command:
```bash
drand show share
```
The JSON-formatted output has the following form:
```json
{
  "index" : 1,
  "share" : {
    "gid": 22,
    "scalar": "764f6e3eecdc4aba8b2f0119e7b2fd8c35948bf2be3f87ebb5823150c6065764"
  }
}
```

The "gid" simply indicates which group the data belongs to. It is present for
scalar and points on the curve, even though scalars are the same on the three
groups of BN256. The field is present already to be able to accommodate
different curves later on.

#### Distributed Key
To retrieve the collective key of the drand beacon our node is involved in,
run:
```bash
drand show cokey
```

### Using Drand
A drand beacon provides several public services to clients. A drand node
exposes its public services on a gRPC endpoint as well as a REST JSON endpoint,
on the same port. The latter is especially useful if one wishes to retrieve
randomness from a JavaScript application.  Communication is protected through
TLS by default. If the contacted node is using a self-signed certificate, the
client can use the `--tls-cert` flag to specify the server's certificate.

### Fetching Public Randomness
To get the latest public random value, run
```bash
drand get public --round <i> <group.toml>
```
where `<group.toml>` is the group identity file of a drand node. You can
specify the round number when the public randomness has been generated. If not
specified, this command returns the most recent random beacon.

The JSON-formatted output produced by drand is of the following form:
```json
{
    "round": 2,
d8a74d4d3b3664a90409f7ec575f7211f06502001561b00e036d0fbd42d2b",
    "signature": "357562670af7e67f3534f5a5a6e01269f3f9e86a7b833591b0ec2a51faa7c11111
2a1dc1baea73926c1822bc5135469cc1c304adc6ccc942dac7c3a52977a342",
    "previous_signature": "5e59b03c65a82c9f2be39a7fd23e8e8249fd356c4fd7d146700fc428ac80ec3f7a22a1dc1baea73926c1822bc5135469cc1c304adc6ccc942dac7c3a52977a342",
    "randomness": "ee9e1aeba4a946ce2ac2bd42ab04439c959d8538546ea637418394c99c522eec2
    92bbbfac2605cbfe3734e40a5d3cc762428583b243151b2a84418e376ea0af6"
}
```

Here `Signature` is the threshold BLS signature on the previous signature value
`Previous` and the current round number. `Randomness` is the hash of
`Signature`, to be used as the random value for this round. The field `Round`
specifies the index of `Randomness` in the sequence of all random values
produced by this drand instance. The **message signed** is therefore the
concatenation of the round number treated as a `uint64` and the previous
signature. At the moment, we are only using BLS signatures on the BN256 curves
and the signature is made over G1.

#### Fetching Private Randomness
To get a private random value, run the following:

```bash
drand get private group.toml
```
The JSON-formatted output produced by drand should look like the following:

```bash
{
    "Randomness": "764f6e3eecdc4aba8b2f0119e7b2fd8c35948bf2be3f87ebb5823150c6065764"
}
```

The command outputs a 32-byte hex-encoded random value generated from the local
randomness engine of the contacted server. If the encryption is not correct,
the command outputs an error instead.

#### Using HTTP endpoints
One may want get the distributed key or public randomness by issuing a GET to a
HTTP endpoint instead of using a gRPC client. Here is a basic example on how to
do so with curl.

To get the distributed key, you can use:
```bash
curl <address>/api/info/distkey
```

Similarly, to get the latest round of randomness from the drand beacon, you can
use
```bash
curl <address>/api/public
```

**All the REST endpoints are specified in the `protobuf/drand/client.proto`
file.**

### Updating Drand Group

Drand allows for "semi-dynamic" group update with a *resharing* protocol that
offers the following:

+ new nodes can join an existing group and get new shares. Note that, in fact,
  all nodes get *new* shares after running the resharing protocol.
+ nodes can leave their current group. It may be necessary for nodes that do
  not wish to operate drand anymore.
+ nodes can update the threshold associated with their current distributed
  public key.

The main advantage of this method is that the distributed public key stays the
*same* even with new nodes coming in. That can be useful when the distributed
public key is embedded inside the application using drand, and hence is
difficult to update.

**Setting up the coordinator**: The coordinator must be a member of the current
network. To run the coordinator, run the following:
```
drand share --leader --transition --nodes 15 --treshold 10 --secret mysecret2 --out
group2.toml
```

**Setting up the current members for the resharing**: The current members can
simply run the following command:
```
drand share --transition --nodes 15 --threshold 10 --secret mysecret2 --out
group2.toml
```

**Setting up the new members**: The new members need the current group file to
proceed. Check how to get the group file in the "Using the drand daemon"
section. Then run the command:
```
drand share --from group.toml --nodes 15 --threshold 10 --secret mysecret2 --out
group2.toml
```

After the protocol is finished, each node will have the new group file written
out as `group2.toml`. The randomness generation starts only at the specified
transition time specified in the new group file.

## Metrics

The `--metrics <metrics-port>` flag may be used to launch a metrics server at
the given port serving [pprof](https://golang.org/pkg/net/http/pprof/) runtime
profiling data at `<metrics-port>/debug/pprof` and
[prometheus](https://prometheus.io/docs/guides/go-application/) metrics at
`<metrics-port>:/metrics`. Prometheus counters track the number of gRPC
requests sent and received by the drand node, as well as the number of HTTP API
requests. This endpoint should not be exposed publicly. If desired, prometheus
metrics can be used as a data source for [grafana
dashboards](https://grafana.com/docs/grafana/latest/features/datasources/prometheus/)
or other monitoring services.

## DrandJS

To facilitate the use of drand's randomness in JavaScript-based applications,
we provide [DrandJS](https://github.com/drand/drandjs). The main method
`fetchAndVerify` of this JavaScript library fetches from a drand node the
latest random beacon generated and then verifies it against the distributed
key.  For more details on the procedure and instructions on how to use it,
refer to the
[readme](https://github.com/PizzaWhisperer/drandjs/blob/master/README.md).

Note this library is still a proof of concept and uses a rather slow pairing
based library in JavaScript.

## Documentation
Here is a list of all documentation related to drand: 

* For a high level presentation of motivations and background, here are some public
  [slides](https://docs.google.com/presentation/d/1t2ysit78w0lsySwVbQOyWcSDnYxdOBPzY7K2P9UE1Ac/edit?usp=sharing)
  about drand or online [video](https://www.youtube.com/watch?v=6BgT-T0wA3U&list=PLhuBigpl7lqu6xWpiXtbEzJQtlMH1tqoG&index=2&t=0s).
* The client-side API documentation of of drand:
  [link](https://hackmd.io/@nikkolasg/HJ9lg5ZTE) 
* The drand *operator guide* documentation:
  [link](https://hackmd.io/@nikkolasg/Hkz2XFWa4) 
* A basic explainer of the cryptography behind drand:
  [link](https://hackmd.io/@nikkolasg/HyUAgm234), 

As well, here is a list of background readings w.r.t to the cryptography used in
drand:

- [Pairing-based
  cryptography](https://en.wikipedia.org/wiki/Pairing-based_cryptography) and
  [Barreto-Naehrig curves](https://github.com/dfinity/bn).
- [Pedersen's distributed key generation
  protocol](https://link.springer.com/article/10.1007/s00145-006-0347-3) for
  the setup.
- Threshold [BLS
  signatures](https://www.iacr.org/archive/asiacrypt2001/22480516.pdf) for the
  generation of public randomness.
- The resharing scheme used comes from the
  [paper](http://citeseerx.ist.psu.edu/viewdoc/download?doi=10.1.1.55.2968&rep=rep1&type=pdf)
  from  Y. Desmedt and S. Jajodia.
- [ECIES](https://en.wikipedia.org/wiki/Integrated_Encryption_Scheme) for the
  encryption of private randomness.

Note that drand was originally a [DEDIS](https://dedis.ch)-owned project that
is now spinning off on its own Github organization. For related previous work
on public randomness, see DEDIS's academic paper [Scalable Bias-Resistant
Distributed Randomness](https://eprint.iacr.org/2016/1067.pdf).

## What's Next?
Although being already functional, drand is still at an early development stage
and there is a lot left to be done. The list of opened
[issues](https://github.com/dedis/drand/issues) is a good place to start. On top
of this, drand would benefit from higher-level enhancements such as the
following:

+ Implement a more [failure-resilient DKG
  protocol](https://eprint.iacr.org/2012/377.pdf) or an approach based on
  verifiable succinct computations (zk-SNARKs, etc).
+ Use / implement a faster pairing based library in JavaScript
+ implement "customizable" randomness, where input is chosen from the user
  (drand would be acting as a distributed threshold
  [oPRF](https://eprint.iacr.org/2018/733.pdf))
+ expand the network
+ implemented ECIES private randomness in JavaScript (?)
+ Add more unit tests
+ Reduce size of Docker
+ Add a systemd unit file
+ Support multiple drand instances within one node

Feel free to submit feature requests or, even better, pull requests ;) But
please note like, this is still currently a side project! Contact me on
[twitter](https://twitter.com/nikkolasg1) for more information about the
project.

## License
The drand source code is released under MIT license originated at the DEDIS lab,
see the file [LICENSE](https://github.com/dedis/drand/blob/master/LICENSE) for
the full text. All modifications brought to this repository are as well under
an MIT license.

## Contributors
Here's the list of people that contributed to drand:

- Nicolas Gailly ([@nikkolasg1](https://twitter.com/nikkolasg1))
- Philipp Jovanovic ([@daeinar](https://twitter.com/daeinar))
- Mathilde Raynal ([@PizzaWhisperer](https://github.com/PizzaWhisperer))
- Ludovic Barman ([@Lbarman](https://github.com/lbarman/))
- Gabbi Fisher ([@gabbifish](https://github.com/gabbifish))
- Linus Gasser ([@ineiti](https://github.com/ineiti))
- Jeff Allen ([@jeffallen](https://github.com/jeffallen))

## Acknowledgments
Thanks to [@herumi](https://github.com/herumi) for providing support on his
optimized pairing-based cryptographic library used in the first version.

Thanks to Apostol Vassilev for its interest in drand and the extensive and
helpful discussions on the drand design.

Thanks to [@Bren2010](https://github.com/Bren2010) and
[@grittygrease](https://github.com/grittygrease) for providing the native
Golang bn256 implementation and for their help in the design of drand and
future ideas.

Finally, a special note for Bryan Ford from the [DEDIS lab](https://dedis.ch)
for letting me work on this project and helping me grow it.

## Coverage

- EPFL blog [post](https://actu.epfl.ch/news/epfl-helps-launch-globally-distributed-randomness-/)
- Cloudflare crypto week [introduction
  post](https://new.blog.cloudflare.com/league-of-entropy/) and the more
  [technical post](https://new.blog.cloudflare.com/inside-the-entropy/).
- Kudelski Security blog
  [post](https://research.kudelskisecurity.com/2019/06/17/league-of-entropy/)
- OneZero
  [post](https://onezero.medium.com/the-league-of-entropy-is-making-randomness-truly-random-522f22ce93ce)
  on the league of entropy
- SlashDot
  [post](https://science.slashdot.org/story/19/06/17/1921224/the-league-of-entropy-forms-to-offer-acts-of-public-randomness)
- Duo
  [post](https://duo.com/decipher/the-league-of-entropy-forms-to-offer-acts-of-public-randomness)
- [Liftr](https://liftrinsights.com/liftr-cloud-look-ahead-cloudflare-introduces-the-league-of-entropy-googles-solution-to-keep-data-sets-private-and-more/)
- (French)
  [nextimpact](https://www.nextinpact.com/brief/cloudflare-presente-la-league-of-entropy--pour-obtenir-des-nombres-aleatoires-9074.html)

## Supporting 
Drand is an open source project, currently as a side project. If you believe in
the project, your financial help would be very valuable. Please contact me on
[twitter](https://twitter.com/nikkolasg1) to know more about the project and
its continuation and how to fund it. More documentation on that front will
arrive.
