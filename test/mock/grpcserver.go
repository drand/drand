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
	testnet "github.com/drand/drand/test/net"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/sign/tbls"
	"github.com/drand/kyber/util/random"
)

// MockService provides a way for clients getting the service to be able to call
// the EmitRand method on the mock server
type MockService interface {
	EmitRand(bool)
}

// Server fake
type Server struct {
	addr string
	*testnet.EmptyServer
	l          sync.Mutex
	stream     drand.Public_PublicRandStreamServer
	streamDone chan error
	d          *Data
	chainInfo  *drand.ChainInfoPacket
}

func newMockServer(d *Data) *Server {
	return &Server{
		EmptyServer: new(testnet.EmptyServer),
		d:           d,
		chainInfo: &drand.ChainInfoPacket{
			Period:      uint32(d.Period.Seconds()),
			GenesisTime: int64(d.Genesis),
			PublicKey:   d.Public,
		},
	}
}

// ChainInfo implements net.Service
func (s *Server) ChainInfo(context.Context, *drand.ChainInfoRequest) (*drand.ChainInfoPacket, error) {
	return s.chainInfo, nil
}

// PublicRand implements net.Service
func (s *Server) PublicRand(c context.Context, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	s.l.Lock()
	defer s.l.Unlock()
	prev := decodeHex(s.d.PreviousSignature)
	signature := decodeHex(s.d.Signature)
	if s.d.BadSecondRound && in.GetRound() == uint64(s.d.Round) {
		signature = []byte{0x01, 0x02, 0x03}
	}
	randomness := sha256Hash(signature)
	resp := drand.PublicRandResponse{
		Round:             uint64(s.d.Round),
		PreviousSignature: prev,
		Signature:         signature,
		Randomness:        randomness,
	}
	s.d = nextMockData(s.d)
	return &resp, nil
}

// PublicRandStream is part of the public drand service.
func (s *Server) PublicRandStream(req *drand.PublicRandRequest, stream drand.Public_PublicRandStreamServer) error {
	s.l.Lock()
	s.streamDone = make(chan error, 1)
	s.stream = stream
	s.l.Unlock()

	err := <-s.streamDone
	s.l.Lock()
	s.stream = nil
	s.l.Unlock()
	return err
}

// EmitRand will cause the next round to be emitted by a previous call to `PublicRandomStream`
func (s *Server) EmitRand(closeStream bool) {
	s.l.Lock()
	if s.stream == nil {
		fmt.Println("MOCK SERVER: stream nil")
		s.l.Unlock()
		return
	}
	stream := s.stream
	done := s.streamDone
	s.l.Unlock()
	if closeStream {
		close(done)
		fmt.Println("MOCK SERVER: closing stream upon request")
		return
	}

	if err := stream.Context().Err(); err != nil {
		done <- err
		fmt.Println("MOCK SERVER: context error ", err)
		return
	}
	resp, err := s.PublicRand(s.stream.Context(), &drand.PublicRandRequest{})
	if err != nil {
		done <- err
		fmt.Println("MOCK SERVER: public rand err:", err)
		return
	}
	if err = stream.Send(resp); err != nil {
		done <- err
		fmt.Println("MOCK SERVER: stream send error:", err)
		return
	}
	fmt.Println("MOCK SERVER: emit round done")
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
	secret            kyber.Scalar
	Public            []byte
	Signature         string
	Round             int
	PreviousSignature string
	PreviousRound     int
	Genesis           int64
	Period            time.Duration
	BadSecondRound    bool
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
	period := time.Second
	d := &Data{
		secret:            secret,
		Public:            publicBuff,
		Signature:         hex.EncodeToString(sig),
		PreviousSignature: hex.EncodeToString(previous[:]),
		PreviousRound:     int(prevRound),
		Round:             round,
		Genesis:           time.Now().Add(period * 1969 * -1).Unix(),
		Period:            period,
		BadSecondRound:    true,
	}
	return d
}

// nextMockData generates a valid Data for the next round when given the current round data.
func nextMockData(d *Data) *Data {
	previous := decodeHex(d.PreviousSignature)
	msg := sha256Hash(append(previous[:], roundToBytes(d.Round+1)...))
	sshare := share.PriShare{I: 0, V: d.secret}
	tsig, err := key.Scheme.Sign(&sshare, msg)
	if err != nil {
		panic(err)
	}
	tshare := tbls.SigShare(tsig)
	sig := tshare.Value()
	return &Data{
		secret:            d.secret,
		Public:            d.Public,
		Signature:         hex.EncodeToString(sig),
		PreviousSignature: hex.EncodeToString(previous[:]),
		PreviousRound:     d.Round,
		Round:             d.Round + 1,
		Genesis:           d.Genesis,
		Period:            d.Period,
		BadSecondRound:    d.BadSecondRound,
	}
}

// NewMockGRPCPublicServer creates a listener that provides valid single-node randomness.
func NewMockGRPCPublicServer(bind string, badSecondRound bool) (net.Listener, net.Service) {
	d := generateMockData()
	testValid(d)
	d.BadSecondRound = badSecondRound
	server := newMockServer(d)
	listener, err := net.NewGRPCListenerForPrivate(context.Background(), bind, "", "", server, true)
	if err != nil {
		panic(err)
	}
	server.addr = listener.Addr()
	return listener, server
}

// NewMockServer creates a server interface not bound to a newtork port
func NewMockServer(badSecondRound bool) net.Service {
	d := generateMockData()
	testValid(d)
	d.BadSecondRound = badSecondRound
	server := newMockServer(d)
	return server
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

// NewMockBeacon provides a random beacon and the chain it validates against
func NewMockBeacon() (*drand.ChainInfoPacket, *drand.PublicRandResponse) {
	d := generateMockData()
	s := newMockServer(d)
	c, _ := s.ChainInfo(context.Background(), nil)
	r, _ := s.PublicRand(context.Background(), &drand.PublicRandRequest{Round: 1})

	return c, r
}
