# Security Model of drand

## Notations

**Drand node**: a node that is running the drand daemon and participating to the
creation of the randomness

**Relay node**: a node that is connected to a drand daemon and exposing a
Internet-facing interface allowing to fetch the public randomness.

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

## Randomness generation model


## Attacks & Practical Remediations


