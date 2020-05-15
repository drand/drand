package mock

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/sign/tbls"
	"github.com/drand/kyber/util/random"
)

// Server fake
type Server struct {
	addr string
	*net.EmptyServer
	l sync.Mutex
	d *Data
}

func newMockServer(d *Data) *Server {
	return &Server{
		EmptyServer: new(net.EmptyServer),
		d:           d,
	}
}

// Group implements net.Service
func (s *Server) Group(context.Context, *drand.GroupRequest) (*drand.GroupPacket, error) {
	return &drand.GroupPacket{
		Threshold:   1,
		Period:      60,
		GenesisTime: uint64(s.d.Genesis),
		DistKey:     [][]byte{s.d.Public},
		Nodes: []*drand.Node{
			{
				Index: 0,
				Public: &drand.Identity{
					Address: s.addr,
					Key:     s.d.Public,
					Tls:     false,
				},
			}},
	}, nil
}

// PublicRand implements net.Service
func (s *Server) PublicRand(c context.Context, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	s.l.Lock()
	defer s.l.Unlock()
	prev := decodeHex(s.d.PreviousSignature)
	signature := decodeHex(s.d.Signature)
	if in.GetRound() == uint64(s.d.Round+1) {
		signature = []byte{0x01, 0x02, 0x03}
	}
	randomness := sha256Hash(signature)
	resp := drand.PublicRandResponse{
		Round:             uint64(s.d.Round),
		PreviousSignature: prev,
		Signature:         signature,
		Randomness:        randomness,
	}
	s.d.Round++
	return &resp, nil
}

// PublicRandStream is part of the public drand service.
func (s *Server) PublicRandStream(req *drand.PublicRandRequest, stream drand.Public_PublicRandStreamServer) error {
	done := make(chan error, 1)

	go func() {
		for {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			defer func() { done <- stream.Context().Err() }()
			select {
			case <-stream.Context().Done():
				return
			case <-ticker.C:
				resp, err := s.PublicRand(stream.Context(), req)
				if err != nil {
					done <- err
					return
				}
				if err = stream.Send(resp); err != nil {
					done <- err
					return
				}
			}
		}
	}()
	return <-done
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
	msg := sha256Hash(append(prev, roundToBytes(d.Round)...))
	if err := key.Scheme.VerifyRecovered(pubPoint, msg, sig); err != nil {
		panic(err)
	}
	invMsg := sha256Hash(append(prev, roundToBytes(d.Round-1)...))
	if err := key.Scheme.VerifyRecovered(pubPoint, invMsg, sig); err == nil {
		panic("should be invalid signature")
	}
	//fmt.Println("valid signature")
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
	Genesis           int64
}

func generateMockData() *Data {
	secret := key.KeyGroup.Scalar().Pick(random.New())
	public := key.KeyGroup.Point().Mul(secret, nil)
	var previous [32]byte
	if _, err := rand.Reader.Read(previous[:]); err != nil {
		panic(err)
	}
	round := 1969
	prevRound := uint64(1968)
	msg := sha256Hash(append(previous[:], roundToBytes(round)...))
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
		Genesis:           time.Now().Unix(),
	}
	return d
}

// NewMockGRPCPublicServer creates a listener that provides valid single-node randomness.
func NewMockGRPCPublicServer(bind string) (net.Listener, net.Service) {
	d := generateMockData()
	testValid(d)
	server := newMockServer(d)
	listener, err := net.NewGRPCListenerForPrivate(context.Background(), bind, server)
	if err != nil {
		panic(err)
	}
	server.addr = listener.Addr()
	return listener, server
}

func sha256Hash(in []byte) []byte {
	h := sha256.New()
	h.Write(in)
	return h.Sum(nil)
}

func roundToBytes(r int) []byte {
	var buff bytes.Buffer
	binary.Write(&buff, binary.BigEndian, uint64(r))
	return buff.Bytes()
}
