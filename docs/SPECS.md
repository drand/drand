# Drand Specifications

## Notation

### Drand node

A drand node is a server that runs the drand code, that participates in the
distributed key generations phases, in the randomness generation and that can
reply to public request API. A Go struct representation is as follow:
```go
type Node struct {
	Key  []byte // public key
	Addr string // publicly reachable address of the node
	TLS  bool // reachable via TLS 
    Index  uint32 // index of the node w.r.t. to the network
}
```
A node can be referenced by its hash as follows:
```go
func (n *Node) Hash() []byte {
    h := blake2b.New(nil)
    h.Write([]byte(n.Addr))
	binary.Write(h, binary.LittleEndian, n.Index)
    h.Write(n.Key)
    return h.Sum(nil)
}
```

### Drand beacon

A drand beacon is what the drand network periodically creates and that can be
used to derive the randomness. A beacon contains the signature of the previous
beacon generated, the round of this beacon and signature. See the [beacon
chain](#beacon-chain) section for more information.

### Group configuration

Group configuration: A structure that contains all the necessary information
about nodes that form a drand network:
* Nodes: A list of nodes information that represents all nodes on the network
* Threshold: The number of nodes that are necessary to participate to a
  randomness generation round to produce a new random value
* Period: The period at which the network creates new random value
* GenesisTime: An UNIX timestamp in seconds that represents the time at which
  the first round of the drand chain starts. See the [beacon
  chain](#beacon-chain) section for more information. 
* GenesisSeed: A generic slice of bytes that is the input for nodes that create
  the first beacon. This seed is the hash of the initial group configuration, as
  shown below.
* Distributed public key: the public key which must be used to verify the
  random beacons created by the network. This field is nil if the network hasn't
  ran the setup phase yet.
* TransitionTime: An UNIX timestamp in seconds that represents the time the
  network denoted by this group configuration took over a previous network. This
  field is empty is the network has never reshared yet. See TODO for more
  information.

#### Group Configuration Hash

The group configuration can be uniquely referenced via its canonical hash. 
The hash is derived using the blake2b hash function.
The Go procedure works as follow:
```
func (g *Group) Hash() []byte {
    h, _ := blake2b.New256(nil)
    // sort all nodes entries by their index
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Index < nodes[j].Index
	})
	// all nodes public keys and positions
	for _, n := range nodes {
		h.Write(n.Hash())
	}
	binary.Write(h, binary.LittleEndian, uint32(g.Threshold))
	binary.Write(h, binary.LittleEndian, uint64(g.GenesisTime))
	if g.TransitionTime != 0 {
		binary.Write(h, binary.LittleEndian, g.TransitionTime)
	}
	if g.PublicKey != nil {
		h.Write(g.PublicKey.Hash())
	}
	return h.Sum(nil)
}
```

## Drand Curve

Drand uses the pairing curve
[BLS12-381](https://hackmd.io/@benjaminion/bls12-381). The hash-to-curve
algorithm is derived from the [RFC
v7](https://tools.ietf.org/html/draft-irtf-cfrg-hash-to-curve-07). 
All points on the curve are sent using the compressed form.
The implementation that drand uses is located in the
[bls12-381](https://github.com/drand/bls12-381) repo.

## Wireformat & API

Drand currently uses [gRPC](https://grpc.io/) as the networking protocol. All
exposed services and protobuf definitions are in the
[protocol.proto](https://github.com/drand/drand/blob/master/protobuf/drand/protocol.proto)
file for the intra-nodes protocols and in the
[api.proto](https://github.com/drand/drand/blob/master/protobuf/drand/api.proto)
file.

## Drand Modules

Generating public randomness is the primary functionality of drand. Public
randomness is generated collectively by drand nodes and publicly available. The
A drand network is composed of a distributed set of nodes and has two
phases / modules:

* Setup: The nodes perform a distributed key generation (DKG) protocol to create
  the collective public key and one private key share per node. The participants
  never see/use the actual (distributed) private key explicitly but instead
  utilize their respectiveprivate key shares for the generation of public
  randomness.
* Generation: After the setup, the nodes switch to the randomness generation
  mode. Each node periodically broadcasts a partial signature to all the other
  participants sign using a t-of-n threshold version of the Boneh-Lynn-Shacham
  (BLS) signature scheme with their respective private key shares. Once any node
  (or third-party observer) has gathered t partial signatures, it can
  reconstruct the full BLS signature, that can be verified against the
  distributed public key.  The signature is then simply the hash of that
  signature, to ensure that there is no bias in the byte representation of the
  final output. 

### Setup phase

To setup a new network, drand uses the notion the of a _coordinator_ that
collects the public key of the participants, creates the group configuration
once all keys are received, push it back to participants and then start the
distributed key generation phase. The coordinator is a member of the new group
by default. At this stage, the coordinator is trusted for setting up the group
configuration and for starting the Distributed Key Generation.

This setup phase uses the notion of a common _secret_ between all participants.
That way, only the participants that know the same secret are able to be listed
in the new group configuration.

#### Collecting the keys of the participants

Each non-coordinator participant sends their information via the following RPC:
```protobuf
rpc SignalDKGParticipant(SignalDKGPacket) returns (drand.Empty);

// SignalDKGPacket is the packet nodes send to a coordinator that collects all
// keys and setups the group and sends them back to the nodes such that they can
// start the DKG automatically.
message SignalDKGPacket {
    // the cryptographic key of the node as well as its address
    Identity node = 1;
    // following parameters are helpful if leader and participants did not run
    // with the same parameters, so the leader can reply if it's not consistent.
    uint32 expected = 2;
    uint32 threshold = 3;
    uint64 dkg_timeout = 4;
    string secret_proof = 5;
    // In resharing cases, previous_group_hash is the hash of the previous group.
    // It is to make sure the nodes build on top of the correct previous group.
    bytes previous_group_hash = 6;
    // XXX uint32 period could be added to make sure nodes agree on the beacon
    // frequency but it's not bringing real security on the table so leave it
    // for now.
}
```

#### Coordinator pushing the new group configuration

Once the coordinator has received the expected number of node informations, then
he creates the group configuration (the operator has given the parameters such as
threshold and period to the drand logic). 
When creating a group from only public keys and addresses of the node, the
_index_ of a node is determined by the lexigraphical order of the public keys as
slice of bytes.
The coordinator then pushes the group configuration to the participants via the
following RPC call:
```protobuf
rpc PushDKGInfo(DKGInfoPacket) returns (drand.Empty);

// PushDKGInfor is the packet the coordinator sends that contains the group over
// which to run the DKG on, the secret proof (to prove it's he's part of the
// expected group, and it's not a random packet) and as well the time at which
// every node should start the DKG.
message DKGInfoPacket {
    drand.GroupPacket new_group = 1;
    string secret_proof = 2;
}
// GroupPacket represents a group that is running a drand network (or is in the
// process of creating one or performing a resharing).
message GroupPacket {
    repeated Node nodes = 1; 
    uint32 threshold = 2;
    // period in seconds
    uint32 period = 3;
    uint64 genesis_time = 4;
    uint64 transition_time = 5;
    bytes genesis_seed = 6;
    repeated bytes dist_key = 7;
}
```

As soon as a participant receives this information from the coordinator, then he
must be ready to accept DKG packets, but he does not start immediatly sending
his packet.  After the coordinator has successfully sent the group to all
participants, he starts sending the first packet of the distributed key
generation. All nodes that receive the first packet of the DKG from the
coordinator (or else, due to network shifts) must send their first packet of the
DKG as well and start the ticker as explained below.

### Distributed Key Generation

The distributed key generation protocol implements the Pedersen's protocol, best
described from [Gennaro's paper](https://www.researchgate.net/publication/225722958_Secure_Distributed_Key_Generation_for_Discrete-Log_Based_Cryptosystems):
> Distributed key generation (DKG) is a main component of threshold cryptosystems. It allows a set of n servers to generate jointly a pair of public and private keys without assuming any trusted party. A DKG may be run in the presence of a malicious adversary who corrupts a fraction (or threshold) of the parties and forces them to follow an arbitrary protocol of their choice. 

**Network level packets**: For a new setup, nodes exchanges the DKG packets
using the following RPC call:
```protobuf
rpc FreshDKG(DKGPacket) returns (drand.Empty);

message DKGPacket {
    dkg.Packet dkg = 1;
}
// dkg.proto
// Packet is a wrapper around the three different types of DKG messages
message Packet {
    oneof Bundle {
        DealBundle deal = 1;
        ResponseBundle response = 2;
        JustifBundle justification = 3;
    }
    bytes signature = 4;
}
```
All messages of the DKG have a canonical hash representation and each node signs
that hash before sending out the packet, therefore providing authentication of
the messages. The signature scheme is the regular BLS signature as explained in
the [cryptography](#cryptography) section.

#### Phase transitions

The protocol runs in _at most_ 3 phases: `DealPhase`, `ResponsePhase` and
`JustificationPhase`. The `FinishPhase` is an additiona local phase where nodes
compute their local private share. However, it can finish after the first two
phases if there is malicious interference or offline nodes during the first
phase.

The way the protocol transition works is via time-outs. As soon as a node starts
the DKG protocol, it starts a ticker that triggers each transition phase.
Example:
* DKG timeout is set to 30s
* Node 1 starts the DKG at time T, so he is in `DealPhase` and sends its deals
  to every other node.
* Node 1's ticker ticks at time T+30s, and node 1 enters the `ResponsePhase` and
  sends its responses to every other node. Each `Response` can be a complaint or a
  success depending on the deal the node received at the previous step.
* Node 1's ticker ticks at time T+60s, and node 1 enters the
  `JustificationPhase` _if there was no complaint response received_ OR in
  `FinishPhase` otherwise.

**Fast Sync**: Drand uses a *fast sync* mode that allows to make the setup
phase proceeds faster at the cost of higher bandwidth usage. Given the
relatively low size of the network, the latter is not a concern. The general
idea is to move to the next step before the ticker kicks in if we received the
messages of the phase from all other nodes already. In more details:
* nodes go into the `ResponsePhase` as soon as they received deals from
  everybody else OR when the ticker kicks in.
* nodes go into the `FinishPhase` as soon as they received "success" responses
  from all other nodes (i.e. all deals were correct) OR
* nodes go into the `JustificationPhase` as soon as they received all responses
  from all other nodes, where at least one of the responses is a complaint. 
* The transition from the `JustificationPhase` to the `FinishPhase` is done
  locally: when a node received all justifications or when the ticker kicks in,
  the nodes compute their final share. The beacon chain is starting at a
  pre-defined time so it doesn't impact how nodes are handling this last phase.

The phases and the respective messages are described in more details in the
following sections.

#### Deal Phase

In this first phase, nodes sends their "deal" containing their encrypted share
to the other nodes as well as the public polynomial from which those shares are
derived. The share is encrypted via ECIES using the public key of the recipient
share holder.  A node bundles all its deals into a `DealBundle` that is
signed.Here is the protobuf wire specification of the deals:

```protobuf
// DealBundle is a packet issued by a dealer that contains each individual
// deals, as well as the coefficients of the public polynomial he used.
message DealBundle {
    // Index of the dealer that issues these deals
    uint32 dealer_index = 1;
    // Coefficients of the public polynomial that is created from the 
    // private polynomial from which the shares are derived.
    repeated bytes commits = 2;
    // list of deals for each individual share holders.
    repeated Deal deals = 3;
}

// Deal contains a share for a participant.
message Deal {
    uint32 share_index = 1;
    // encryption of the share using ECIES
    bytes encrypted_share = 2;
}
```

Each `DealBundle` is authentificated so a node can know when they received all
expected `DealBundle`, one from each node, by looking at the
`dealer_index` field of all `DealBundle. If that is the case, the node can
directly transition to the `ResponsePhase`. Otherwise, the node needs to wait
until the ticker kicks in for the next timeouts, before entering the
`ResponsePhase`.

#### Response Phase

In this second phase, each node first process all their deals received during
the previous phase. Each nodes then sends a Response for each shares they have
received and _should_ have received: if there is a missing share for a node,
this node will send a response for it as well.  A `Response` contains both the
"share holder" index and the "dealer index" as well as a status. If the share
holder found its share from that dealer invalid, the status is set as a
complaint (`false`) and if the share was valid, the status is set as a success
(`true`).
A node bundles all its responses into a `ResponseBundle` that is signed. Here is
the protobuf description:

```protobuf
// ResponseBundle is a packet issued by a share holder that contains all the 
// responses (complaint and/or success) to broadcast.
message ResponseBundle {
    uint32 share_index = 1;
    repeated Response responses = 2;
}

// Response holds the response that a participant broadcast after having
// received a deal.
message Response {
    // index of the dealer for which this response is for
    uint32 dealer_index = 1;
    // Status represents a complaint if set to false, a success if set to 
    // true.
    bool status = 2;
}
```

When a node received all expected `ResponseBundle` from each node OR when the
ticker kicks in, the node decides to which phase to proceed to:
* If all `Response.status` from each `ResponseBundle` are set to true, the node
  can directly go into the `FinishPhase` and compute their final share.
* If not, then the node needs to go into the `JustificationPhase`.

#### Justification Phase

For each "complaint" responses (i.e. `status == false`) whose`dealer_index` is
equal to their index, a node sends a `Justification` packet that contains the
non-encrypted share that the share holder should have received. The goal here is
that every node will be able to verify the validity of the share now that is
unencrypted.
A node bundles all its `Justifications` into a `JustificationBundle` that is
signed. Here is the protobuf description:

```protobuf
// JustifBundle is a packet that holds all justifications a dealer must 
// produce
message JustifBundle {
    uint32 dealer_index = 1;
    repeated Justification justifications = 2;
}

// Justification holds the justification from a dealer after a participant
// issued a complaint response because of a supposedly invalid deal.
message Justification {
    // represents for who share holder this justification is
    uint32 share_index = 1;
    // plaintext share so everyone can see it correct
    bytes share = 2;
}
```
A node can silently waits until it receives all justification expected or the
ticker kicks in to go into the `FinishPhase`.

#### Finish Phase

In the `FinishPhase`, each node locally look at the shares they received and
compute both their final share and the distributed public key. For the DKG to be
sucessful, there must be at least more than a threshold of valid shares. For
more detail, see the [cryptography](#cryptography) section.
Each node must save the group configuration file augmented with the distributed
key. This configuration file is now representative of functional current drand network.

Each node must start generating randomness at the time specified in the
`GenesisTime` of the group configuration file.

### Randomness generation 

#### Overiew

The randomness generation protocol works in its simple form by having each node
periodically broadcasts a "partial" signature over a common input. Each nodes
waits to receive these partial signatures, and as soon as one has a subset of at
least a "threshold" (parameter given in the group configuration file) of those,
this node can reconstruct the final signature. The final signature is a regular
BLS signature that can be verified against the distributed public key. If that
signature is correct, then the randomness is simply the hash of it: 
```go
rand := sha256.Sum256(signature)
```
Note here that the hash function shown here is simply an example, a suggestion.
An application is free to hash the signature using any secure hash function. The
important point is to verify to validity of the signature.

#### Randomness Generation Period

The drand network outputs a new random beacon every period and associates a
beacon "round" to a specific time. The mapping between a time and a round allows
to exactly determine the round number for any given time in the past or future.
The relation to determine this mapping is as follow:
```go
// Parameters:
// * now: UNIX timestamp in seconds
// * genesis: UNIX timestamp in seconds
// * period: period in seconds
// Returns:
// * round: the round number at which the drand network is at
// * the time at which this round started
func CurrentRound(now, genesis int64, period uint32) (round uint64, time int64){
	if now < genesis {
        // round 0 is the genesis block: signature is the genesis seed
		return 0
	}
	fromGenesis := now - genesis
	// we take the time from genesis divided by the periods in seconds, that
	// gives us the number of periods since genesis.  We add +1 because round 1 
    // starts at genesis time.
	round = uint64(math.Floor(float64(fromGenesis)/period)) + 1
	time = genesis + int64(nextRound*uint64(period.Seconds()))
    return
}
```

Each node starts sending their partial signature for a given round when it is
time to do so, according to the above function. Given the threat model, there
is always enough honest nodes such that the chain advances at the correct speed.
In case this is not true at some point in time, please refer to the [catchup
section](#catchup) for more information.

#### Beacon Chain

Drand binds the different random beacon together so they form a chain of random
beacons. Remember a drand beacon is structured as follow:
```go
type Beacon struct {
    Round uint64
    PreviousSignature []byte 
    Signature []byte
}
```
* The `Round` is the round at which the beacon was created, as explained in the
  previous section.
* The `PreviousSignature` is the signature of the beacon that was created at
  round `Round - 1`
* `Signature` is the final BLS signature created by aggregating at least
  `Threshold` of partial signatures from nodes.

This structure makes it so that each beacon created is building on the previous
one therefor forming a randomness chain.

**Partial Beacon Creation**: At each new round, a node creates a `PartialBeacon`
with the current round number, the previous signature and the partial signature
over the message: 
```go
func Message(currRound uint64, prevSig []byte) []byte {
	h := sha256.New()
	h.Write(prevSig)
	h.Write(roundToBytes(currRound))
	return h.Sum(nil)
}
```
To determine the "current round" and the "previous signature", the node loads it
last generated beacon and sets the following:
```
currentRound = lastBeacon.Round + 1
previousSignature = lastBeacon.Signature
```
It is important to note that the current round may not be necessarily the round
of the current time. More information in the following section.

**Partial Beacon Broadcast**:Each node then calls the following RPC call with
the following protobuf packet: XXX: protobuf shown is assuming [issue
256](https://github.com/drand/drand/issues/256)is fixed.
```protobuf
rpc PartialBeacon(PartialBeaconPacket) returns (drand.Empty);

message PartialBeaconPacket {
    // Round is the round for which the beacon will be created from the partial
    // signatures
    uint64 round = 1;
    bytes partial_sig = 2;
    // previous sig the signature of the beacon generated at round `round-1`
    bytes previous_sig = 3;
}
```
**Final Beacon Creation**: For each incoming partial beacon packet, a
node must first verify it, using the partial signature verification routine and
then stores it in a temporary cache if it is valid.  As soon as there is at
least a threshold of valid partial signatures, the node can aggregate them to
create the final signature. 

**Validation of beacon and storage**: Once the new beacon is created, the node
verifies its signature, loads the last saved beacom from the database and checks
if the following routine returns true:
```go
func isAppendable(lastBeacon, newBeacon *Beacon) bool {
	return newBeacon.Round == lastBeacon.Round+1 &&
		bytes.Equal(lastBeacon.Signature, newBeacon.PreviousSig)
}
```
There should never be any gaps in the rounds.
A node can now save the beacon locally in its database and exposes it to the
external API.


#### Catchup mode

Nodes must have a ticker that kicks in every period of time (started at the
genesis time). At each kicks, a node loads its last beacon generated and runs
the protocol as shown above. Under normal circumstances, the `Round` field
should be the round that corresponds to the current time.

**Network halting**: However, it may happen that there is not enough partial
beacon being broadcasted at one time therefore there will not be any random
beacon created for this round. Under these circumstances, nodes can enter a
"catchup" mode.

To detect if the network is stalled, each node at each new tick must verify that
the `lastBeacon.Round + 1` equals the current round given by their local clock.
If that is not the case, that means there wasn't a random beacon generated in
time in the previous round OR this node didn't receive enoug partial beacons for
some reasons.

If that condition is true, nodes must first try to sync with each other: each node
asks the other nodes if they have a random beacon at the round `lastBeacon.Round
+ 1` AND higher beacons as well. If a node receives a valid beacon for the
requested round, that means the network is still producing randomness but for
some networking reasons, he didn't receive correctly the partial beacons. In
this case, if the last beacon received corresponds to the current round, the
node must wait on the next tick and continue as usual.

If the sync didn't return any more recent valid beacons, that probably means the
network is stalled. In that case, nodes must continue to broadcast the same
partial beacon as usual at every tick. 

**Network Catchup**: At some point, there will be enough honest & alive nodes to
broadcast their partial signatures such that a new beacon can be aggregated, for
the round R.  However, that round R does not correspond to the current round
given their local clock. In this situation, each node must produce their partial
beacons until the current round as fast as possible.  More concretely, each node
broadcasts their new partial beacon from the last generated beacon until they
reach the current round according to the local clock. As soon as a new beacon is
aggregated, nodes look if the round corresponds to the current round, and if
not, prepare to broadcast their next partial signatures.

#### Syncing

When a drand node is offline, restarted or detects a halt in the chain's
progress (previous section), a node should sync to all other nodes in the
network. A node indicates the last beacon saved in its database and calls the
following RPC:
```protobuf
rpc SyncChain(SyncRequest) returns (stream BeaconPacket);
// SyncRequest is from a node that needs to sync up with the current head of the
// chain
message SyncRequest {
    uint64 from_round = 1;
}

message BeaconPacket {
    bytes previous_sig = 1;
    uint64 round = 2;
    bytes signature = 3;
}
```

**Client side**: For each incoming `BeaconPacket`, the node runs the regular
beacon verification routine as usual. The client stops the syncing process
(closes the RPC call) when the last valid beacon's round returned is equal to
the current round.

**Server side**: For sync request, the node must load the beacon which has the
given round requested and sends back all subsequent beacons until the last one.

## External API

## Control API

## THINGS TO REVIEW

* Setup phase: now it doesn't require any manual downloading from operators, and
  it's a huge win given the manual errors we've seen previously. But the
  coordinator is trusted to setup the group correctly. 
  Given the setup phase is done in a controlled fashion, it hasn't been a been a
  practical problem but we should think about a best practice here.
  One idea is to add an additional step such that a participant can inspect the
  group file and accept or reject it, manually. Depending on that, the node can
  either sign the new group configuration, and when the leader has received
  all the signatures on the group file, he can push that signed group
  configuration again to participants and start the DKG. Another slightly
  different model is to simply say that a participate could refuse to run the
  DKG if the group configuration is deemed invalid.
