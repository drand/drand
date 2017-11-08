package main

import (
	"bytes"
	"encoding/binary"
	"reflect"

	"github.com/dedis/drand/bls"
	"github.com/dedis/protobuf"
	kyber "github.com/dedis/kyber"
	"github.com/dedis/kyber/share/pedersen/dkg"
)

// DrandPacket is the global wrapper of all protocol packets
type DrandPacket struct {
	Hello  *Public       // Hello is used when initiating connection to another peer
	Dkg    *DKGPacket    // DKg holds all dkg-related information
	Beacon *BeaconPacket // Beacon holds all random beacon related information
}

// DKGPacket holds the different structures needed by a round of the DKG protocol
// NOTE: justification is not currently used.
type DKGPacket struct {
	Deal          *dkg.Deal
	Response      *dkg.Response
	Justification *dkg.Justification
}

// BeaconPacket holds the different structures needed by a round of a TBLS
// protocol.
type BeaconPacket struct {
	Request *BeaconRequest
	Reply   *BeaconReply
}

// BeaconRequest contains all information needed by a signer to:
// 1) validate that the previous signature has been generated correctly
//    and form the "chain", be able to store it in files etc.
// 2) create its partial signature to send back to the leader
type BeaconRequest struct {
	PreviousSig []byte // resulting signature of the previous round
	Timestamp   int64  // timestamp to concatenate with PreviousSig = message to sign
}

// Message returns the raw message that is expected to be signed in order to
// produce a BeaconReply. This message is what can be verified by external end
// users.
func (b BeaconRequest) Message() []byte {
	var buff bytes.Buffer
	binary.Write(&buff, binary.LittleEndian, b.Timestamp)
	buff.Write(b.PreviousSig)
	return buff.Bytes()
}

// BeaconReply contains the request and an associated threshold partial
// signature
type BeaconReply struct {
	Request   *BeaconRequest
	Signature *bls.ThresholdSig
}

// unmarshal reads the protobuf encoded buffer into a Drand struct
func unmarshal(g kyber.Group, buff []byte) (*DrandPacket, error) {
	cons := make(protobuf.Constructors)
	var s kyber.Scalar
	var p kyber.Point
	cons[reflect.TypeOf(&s).Elem()] = func() interface{} { return g.Scalar() }
	cons[reflect.TypeOf(&p).Elem()] = func() interface{} { return g.Point() }
	var drand = new(DrandPacket)
	return drand, protobuf.DecodeWithConstructors(buff, drand, cons)
}
