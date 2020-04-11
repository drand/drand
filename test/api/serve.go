package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	json "github.com/nikkolasg/hexjson"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/sign/tbls"
	"github.com/drand/kyber/util/random"
)

const serve = "localhost:1969"

// Server fake
type Server struct {
	*net.EmptyServer
	d *Data
}

func newServer(d *Data) *Server {
	return &Server{
		EmptyServer: new(net.EmptyServer),
		d:           d,
	}
}

// Group implements net.Service
func (s *Server) Group(context.Context, *drand.GroupRequest) (*drand.GroupPacket, error) {
	return &drand.GroupPacket{
		Threshold: 1,
		Period:    60,
		Nodes: []*drand.Identity{&drand.Identity{
			Address: serve,
			Key:     s.d.Public,
			Tls:     false,
		}},
	}, nil
}

// PublicRand implements net.Service
func (s *Server) PublicRand(c context.Context, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	prev := decodeHex(s.d.PreviousSignature)
	signature := decodeHex(s.d.Signature)
	if in.GetRound() == uint64(s.d.Round+1) {
		signature = []byte{0x01, 0x02, 0x03}
	}
	randomness := sha256Hash(signature)
	return &drand.PublicRandResponse{
		Round:             uint64(s.d.Round),
		PreviousSignature: prev,
		PreviousRound:     uint64(s.d.PreviousRound),
		Signature:         signature,
		Randomness:        randomness,
	}, nil
}

// DistKey implements net.Service
func (s *Server) DistKey(context.Context, *drand.DistKeyRequest) (*drand.DistKeyResponse, error) {
	fmt.Println("distkey called")
	return &drand.DistKeyResponse{
		Key: s.d.Public,
	}, nil
}

func testValid(d *Data) {
	pub := d.Public
	pubPoint := key.KeyGroup.Point()
	if err := pubPoint.UnmarshalBinary(pub); err != nil {
		panic(err)
	}
	sig := decodeHex(d.Signature)
	prev := decodeHex(d.PreviousSignature)
	msg := beacon.Message(prev, uint64(d.PreviousRound), uint64(d.Round))
	if err := key.Scheme.VerifyRecovered(pubPoint, msg, sig); err != nil {
		panic(err)
	}
	invMsg := beacon.Message(prev, uint64(d.PreviousRound), uint64(d.Round-1))
	if err := key.Scheme.VerifyRecovered(pubPoint, invMsg, sig); err == nil {
		panic("should be invalid signature")
	}
	fmt.Println("valid signature")
	//VerifyRecovered(public kyber.Point, msg, sig []byte) error
}

func decodeHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// Data of signing
type Data struct {
	Public            []byte
	Signature         string
	Round             int
	PreviousSignature string
	PreviousRound     int
}

func generateData() *Data {
	secret := key.KeyGroup.Scalar().Pick(random.New())
	public := key.KeyGroup.Point().Mul(secret, nil)
	var previous [32]byte
	if _, err := rand.Reader.Read(previous[:]); err != nil {
		panic(err)
	}
	round := 1969
	prevRound := uint64(1968)
	msg := beacon.Message(previous[:], prevRound, uint64(round))
	fmt.Println("msg: ", hex.EncodeToString(msg))
	sshare := share.PriShare{I: 0, V: secret}
	tsig, err := key.Scheme.Sign(&sshare, msg)
	if err != nil {
		panic(err)
	}
	tshare := tbls.SigShare(tsig)
	sig := tshare.Value()
	publicBuff, _ := public.MarshalBinary()
	d := &Data{
		Public:            publicBuff,
		Signature:         hex.EncodeToString(sig),
		PreviousSignature: hex.EncodeToString(previous[:]),
		PreviousRound:     int(prevRound),
		Round:             round,
	}
	//s, _ := json.MarshalIndent(d, "", "    ")
	//fmt.Println(string(s))
	return d
}

type TestJSON struct {
	Public string
	API    *drand.PublicRandResponse
}

func main() {
	d := generateData()
	testValid(d)
	server := newServer(d)
	resp, err := server.PublicRand(context.TODO(), &drand.PublicRandRequest{})
	if err != nil {
		panic(err)
	}
	tjson := &TestJSON{
		Public: hex.EncodeToString(d.Public),
		API:    resp,
	}
	s, _ := json.MarshalIndent(tjson, "", "    ")
	fmt.Println(string(s))
	listener := net.NewTCPGrpcListener(serve, server)
	fmt.Println("server will listen on ", serve)
	listener.Start()
}

func sha256Hash(in []byte) []byte {
	h := sha256.New()
	h.Write(in)
	return h.Sum(nil)
}
