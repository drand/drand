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
    h := blake2b.New(nil)
    h.Write([]byte(n.Addr))
	binary.Write(h, binary.LittleEndian, n.Index)
    h.Write(n.Key)
    return h.Sum(nil)
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
```

## Drand Curve

Drand uses the pairing curve
[BLS12-381](https://hackmd.io/@benjaminion/bls12-381). The hash-to-curve
algorithm is derived from the [RFC
v7](https://tools.ietf.org/html/draft-irtf-cfrg-hash-to-curve-07). 
All points on the curve are sent using the compressed form.
The implementation that drand uses is located in the
[bls12-381](https://github.com/drand/bls12-381) repo.

## Networking & exposed API

Drand currently uses [gRPC](https://grpc.io/) as the networking protocol. All
exposed services and protobuf definitions are in the [protocol.proto](https://github.com/drand/drand/blob/master/protobuf/drand/protocol.proto) file for the intra-nodes protocols and in the [api.proto](https://github.com/drand/drand/blob/master/protobuf/drand/api.proto) file.

## Drand Protocols

Generating public randomness is the primary functionality of drand. Public
randomness is generated collectively by drand nodes and publicly available. The
A drand network is composed of a distributed set of nodes and has two
phases:

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
  collective public key.  The signature is then simply the hash of that
  signature, to ensure that there is no bias in the byte representation of the
  final output.  against the collective public key.

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

After the coordinator has successfully sent the group to all participants, he
starts sending the first packet of the distributed key generation.

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
the messages.

**Phase transitions**: This protocol runs in _at most_ 3 phases: `DealPhase`,
`ResponsePhase` and `JustificationPhase`. However, it can finish after the first
two phases if there is malicious interference or offline nodes during the first
phase. 

The phases are described in the following sections.

#### Deal Phase

Nodes produce their share to send to the other nodes in an encrypted form as
well as the public polynomial from which those shares are derived.

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

#### Response Phase

#### Justification Phase

### Beacon Chain

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
