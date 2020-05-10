# Security Model of drand

## Table of Contents

* [Notations](#notations)
* [Model](#model)
   * [Distributed Key Generation security model](#distributed-key-generation-se
-model)
   * [Randomness generation model](#randomness-generation-model)
* [Attacks](#attacks)
   * [Randomness Generation](#randomness-generation)
      * [DDoS the drand network](#ddos-the-drand-network)
      * [Corruption of the drand network](#corruption-of-the-drand-network)
   * [Distributed Key Generation](#distributed-key-generation)
      * [DDoS attacks](#ddos-attacks)
      * [Corruption attacks](#corruption-attacks)
      * [Broadcast Channel Assumption](#broadcast-channel-assumption)
* [TO REVIEW:](#to-review)

## Notations

**Drand node**: a node that is running the drand daemon and participating to the
creation of the randomness. Each drand node have a longterm public key and a
private share (after running the setup/resharing phase). The drand network is
the set of drand nodes connected with each other.

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

**Authenticated Channel**: Every communication between nodes must be
authenticated. Drand achieves this by signing every outgoing DKG packets with
a BLS signature on the longterm public key of the sender node.

**Public Group**: Every nodes willing to run the DKG must know the group
formation before starting the DKG, including the longterm public keys of each
node. During the DKG, there might be some nodes offline or misbehaving. The set
of nodes that _successfully_ passed the DKG are called the _qualified_ set of
nodes (QUAL). Only these nodes have valid shares and are able to produce
partial beacons that can be successfully validated with respect to the
distributed public key.

**DKG's biasability in signatures**: Pedersen's DKG is known to exhibit a
weakness in the biasability in the distribution of the distributed private key.
However, the same authors (Gennaro et al.) that proved the latter also proved
this bias is not relevant in the setting of using the DKG to perform digital
signatures, which offers other strong properties -
[paper](https://pdfs.semanticscholar.org/642b/d1bbc86c7750cef9fa770e9e4ba86bd49eb9.pdf).
In particular, the paper mentions discrete log based systems. However, it is not
yet strictly proven that system using computation co-CDH assumptions as required
for threshold BLS signatures are secure in the model of Gennaro.  However, it is
believed that this assumptions holds in this context as well and is being worked
on.  Note that using threshold BLS signatures as a source of randomness _is_
formally proven secured in this [paper](https://eprint.iacr.org/2020/096.pdf)
from Galindo et al. 

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

**Resharing**: During a resharing, a drand network A (with threshold
`tA`)creates new shares for a drand network B (with threshold `tB`) which can be
disjoinct from A, such the drand network B is now responsible to continue
creating drand beacons, while that the distributed public key doesn't change.
For this to happen, there needs to be at least `tA` nodes from network A and
`tB` nodes alive and honest during the resharing. At the end of the protocol,
there is gonna be at least `tB` nodes that are qualified and have private shares
to generate randomness.

## Attacks

### Randomness Generation

There can be multiple ways of attacking the drand network during the randomness
generation phase, each with different consequences.

#### DDoS the drand network

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
met (it seems to be quite difficult to put in practice), there still needs to be
at least one drand nodes that is not under attack to aggregate the partial
beacons. To increase the chance of reconstructing the final random beacon from
the partial beacons that "leaks" out from drand nodes under attack, it could
also be possible to setup aggregator nodes. Such nodes could be under heavy
protection, potentially with a more centralized governance, whose job is only
collects the different partial beacons and aggregates them to deliver them to
the relay network. There could be many such aggregators nodes such that the
chance of getting at least one of these received enough threshold beacon
drastically increases.

#### Corruption of the drand network

**Scenario #1**: Corruption of less than threshold of nodes

In this scenario, the attacker corrupts _less_ than a threshold of drand nodes.
_Consequence_: The attacker is _not_ able to derive any meaningful
information with respect to beacon chain (i.e. he can't derive future beacons).
However, it is assumed it has now access to the long term private key of each
compromised node.

**Scenario #2**: Corruption of more than a threshold of nodes

In this scenario, the threat model of drand is now violated and thus is the
scenario to avoid at all costs: the attacker corrupts _at least_ a threshold of
drand nodes.  _Consequence_: The attacker is now able to derive the whole chain,
i.e. it can derive any given random beacon of the chain. The drand randomness is
not _unpredictable_ anymore from the point of view of the attacker.  However,
the drand randomness stays _unbiasable_: attacker is not able to change the
randomness in any way.

**Mitigation**: Proactive resharing allows both to 
1. Let a new group of nodes take over the randomness generation, potentially
   with more nodes and higher threshold. 
2. Refresh shares for nodes that participate in the resharing in the new group:
   partial beacons created from an old share is not validated by the members of
   the new group.

Given a "periodical" resharing with more nodes, it makes it harder for the
attacker to maintain the grasp on the shares of the drand nodes since he must
have continuous control over the drand node itself. If the operator of a
corrupted drand node recovers from the attacker's intrusion, after a resharing,
the attacker's share is invalid. Moreover, a resharing with more nodes highers
the bar for the attacker to attain the second scenario both because of the
previous argument and because attacker needs now to corrupts more nodes than in
the initial group now.

As such, it is recommended to reshare often, _even if_ between the same nodes,
as it creates new shares.

### Distributed Key Generation

#### DDoS attacks

If during the DKG, some nodes are DDoS attacked, then these nodes might not be
able to receive the deals (shares) in time and / or reply in the second phase in
time. Given the necessity of time for achieving the synchronous network
assumption, that means these nodes risk getting excluded from the final group
that gets shares at the end.

#### Corruption attacks

**Scenario 1**: An attacker only "controls" less than a threshold of nodes. The
attacker can choose the private polynomial used to create the shares. Attacker can
influence the distribution of the private share but is believed to not being
able to bias the distribution of the randomness later on. 

**Scenario 2**: An attacker controls more than a threshold of nodes during the
DKG. This scenario is similar to the scenario 2 for the randomness generation
since even before the DKG: attacker can know before the end of the DKG the whole
randomness chain (since he can see the honest shares before sending them).

#### Broadcast Channel Assumption

Attacker is at least one node in the group and broadcasts inconsistent shares
and public polynomial to different parties. Given drand does not use a
_reliable_ broadcast channel, the attacker is able to send any shares over
different polynomials for example - see
[here](https://www.jcraige.com/vss-forgery) for one example of such an attack.
Note attacker could try to partition the set of honest nodes in two such that
each half would have consistent shares within itself but inconsistent with
respect to the other half.

**Consequence**: Shares can be inconsistent with each other, and in such cases,
nodes will not be able to verify partial beacons during the randomness
generation phase. Another more subtil scenario is that nodes finish the DKG
with half of the honest nodes having a distributed public key different than the
other half, a "split".

**Practical Observation**: After a DKG is setup, nodes (1) publish the
distributed public key they have and (2) start the randomness generation rounds.
The first step enables any third party to verify the distributed public keys are
the same (it is in fact sufficient to verify a threshold of them have the same).
In (2), the chain will not be able to advance and therefore it becomes clear
that the DKG step went wrong.  Given the DKG phase is run once in a while, it is
reasonable to assume nodes can restart the DKG phase in case things have gone
wrong.

**Remediation to keep assumption true - TODO**: A practical step towards ensuring non
equivocation during the DKG phase is to move to a gossiping approach.  Indeed,
an attacker that would send different public polynomials is likely to end up as
not a qualified dealer since honest nodes would relay its packet and find the
inconsistency.

## TO REVIEW:

* drand must make sure that if a DKG went "wrong" during a resharing because of
  the broadcast channel assumption being violated, it keeps the previous share
  to be able to start a new DKG again. Currently it erases the previous share
  when DKG finishes. For a fresh DKG it is fine since nodes can restart from
  scratch as before.
