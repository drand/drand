# Security Model of drand

## Notations

**Drand node**: a node that is running the drand daemon and participating to the
creation of the randomness. The drand network is the set of drand nodes
connected with each other.

**Relay node**: a node that is connected to a drand daemon and exposing a
Internet-facing interface allowing to fetch the public randomness. The relay
network is the set of relay nodes, partially / potentially connected with each
other.

When the type of the node is not specified in the document, it is assumed from
the context - most often it refers to a drand node.

**Corrupted node**: a node who is in the control of an attacker. In this case
the attacker has access to all the cryptographic material this node posses as
well as the networking authorization. For example, if a relay node is corrupted,
an attacker has a direct connection to a drand node.

**Offline node**: a node who is unreachable from an external point of view. It
can be offline from the point of view of another drand node or a relay node. The
document tries to clarify in which context when relevant.

**Alive node**: a node which is running the binary and sends packets out to the
inernet.


## Model

In drand, there are two phases which do not require the same security
assumptions. This section highlights both models and the practical realization
or assumptions taken.

### Distributed Key Generation security model

The DKG protocol model follows the one from the Pedersen's protocol. The [paper
description](https://www.researchgate.net/publication/225722958_Secure_Distributed_Key_Generation_for_Discrete-Log_Based_Cryptosystems)
linked is from Gennaro's paper that explains the protocol and its assumptions in
clean way.

**Synchronous Network**: A packet sent from an alive node reaches its
destination in a bounded amount of time. Drand realizes this assumptions by the
usage of timeouts during the DKG protocol.

**Synchronized Clocks**: All nodes must have roughly synchronized clocks. 

**Reliable Broadcast Channel**: When a node broadcasts a packet to all other
nodes, each other node is guaranteed to receive the same exact packet after some
bounded amount of time. This assumption is not strictly realized by drand
currently. See [DKG attacks](#dkg-attacks) section to understand the impact.

### Randomness generation model

**Network**: The randomness generation protocol do not make any assumption on
the network bounds. As soon as packet comes in, node processes them and the
chain advances if conditions are there (enough partial beacon and time for a new round).

**Synchronized Clocks**: All nodes must have roughly synchronized clocks to
start the rounds at the same time. The precision of the synchronicity between
clocks only needs to be at the order the round frequency (order of tens of
seconds), which is much higher than the reality of server's clock (NTP-synced
servers have drifts of under a second over the globe). XXX need source.

**Broadcast channel**: The randomness generation models only needs a regular
broadcast channel. It does **not** need to be reliable given the deterministic
nature of a partial beacon and the drand chain: for a given round, a given node
can only send one valid partial beacon.

**Threshold**: The threshold is the amount of nodes that must be online and
honest at a given time to broadcast their partial signature in order to create
the final random beacon. See the attack section to know what are the
consequences when that is not the case.

**Determinism of the chain**: The chain is deterministic with respect to a fresh
DKG phase. It means, if a trust party gathered a threshold of private shares, it
could generate any beacon of the chain. When a resharing occurs, the individual
shares of each drand node change but the chain remains the same as well. If the
same set of nodes perform a new fresh DKG, it will create a new chain from
scratch.

## Attacks

### Randomness Generation

There can be multiple ways of attacking the drand network during the randomness
generation phase, each with different consequences.

### DDoS Attacks

#### Attacking the drand network

**Scenario**: There is a DDoS attacks on multiple drand nodes and at least a
threshold of honest drand nodes are now considered offline and can't get other's
partial beacons. The attack is substained for a duration X.

**Consequence**: The chain halts for as long as the DDoS attack is substained on
the drand nodes OR for as long as the drand operators didn't move their drand
node to another IP / network not under attacked. That means there will not be
any new drand beacon for the number of rounds contained in X.

**Criteria for success**: The DDoS attack must bring completely down the network
around a threshold of nodes. Completely means there is not a _single_ outgoing
partial beacon that leaves the drand's node network. That is a _critical
distinction_ to make since, otherwise, a drand node could still collect the
partial beacons of under-attacked drand nodes, one by one. As soon as this node
gets a threhsold of them, it can reconstruct the final beacon and broadcast it
to the relay network. 

**Defense mechanism**: To counter DDoS attacks, the drand nodes must block the
incoming traffic as early as possible. To achieve that, allowing traffic only
from other drand nodes based on their IP addresses seems the most efficient way
to deal with DDoS attacks.

**Potential additional defense mechanism**: Assuming the last criteria is not
met, there still needs to be at least one drand nodes that is not under attack
to aggregate the partial beacons. To increase the chance of reconstructing the
final random beacon from the partial beacons that "leaks" out from drand nodes
under attack, it could also be possible to setup aggregator nodes which are
under heavy protection, potentially with a more centralized governance, whose
job is only collects the different partial beacons and aggregates them to
deliver them to the relay network.

#### Attacking the relay network

?? 

### Corruption attacks

#### Corruption of less than threshold of nodes

In this scenario, the attacker gets ahold of the cryptographic material of
_less_ than a threshold of drand nodes.

**Immediate consequence**: The attacker is _not_ able to derive any meaningful
information with respect to beacon chain (i.e. he can't derive future beacons).
However, it is assumed it has now access to the long term private key of each
compromised node.

**Longterm consequence**: 
