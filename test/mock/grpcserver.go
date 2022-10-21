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
	"testing"
	"time"

	"github.com/drand/drand/common/scheme"
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
	EmitRand(*testing.T, bool)
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
			GenesisTime: d.Genesis,
			PublicKey:   d.Public,
			SchemeID:    d.Scheme.ID,
		},
	}
}

// ChainInfo implements net.Service
func (s *Server) ChainInfo(context.Context, *drand.ChainInfoRequest) (*drand.ChainInfoPacket, error) {
	return s.chainInfo, nil
}

// PublicRand implements net.Service
func (s *Server) PublicRand(c context.Context, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	start := time.Now()
	defer func() {
		fmt.Printf("finished server.PublicRand in %s\n", time.Now().Sub(start))
	}()

	fmt.Println("before lock in server.PublicRand")

	s.l.Lock()
	defer s.l.Unlock()

	select {
	case <-c.Done():
		return nil, fmt.Errorf("context closed in public rand")
	default:
		fmt.Println("running server.PublicRand")
	}

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
	start := time.Now()
	defer func() {
		fmt.Printf("finished server.PublicRandStream in %s\n", time.Now().Sub(start))
	}()

	select {
	case <-stream.Context().Done():
		return fmt.Errorf("context closed in public rand stream")
	default:
		fmt.Println("running server.PublicRandStream")
	}

	streamDone := make(chan error)
	s.l.Lock()
	s.streamDone = streamDone
	s.stream = stream
	s.l.Unlock()

	// We want to remove the stream here but not while it's in use.
	// To fix this, we'll defer setting stream to nil and wait for
	// the launched operations to finish, see below.
	defer func() {
		select {
		case <-stream.Context().Done():
			fmt.Println("context closed in public rand stream in defer")
		default:
			fmt.Println("closing server.PublicRandStream")
		}
		s.l.Lock()
		s.stream = nil
		s.l.Unlock()
	}()

	// Wait for values to be sent before returning from this function.
	return <-streamDone
}

// EmitRand will cause the next round to be emitted by a previous call to `PublicRandomStream`
func (s *Server) EmitRand(t *testing.T, closeStream bool) {
	t.Logf("trying to obtain lock on server.EmitRand(%t)\n", closeStream)
	s.l.Lock()
	if s.stream == nil {
		t.Log("MOCK SERVER: stream nil")
		s.l.Unlock()
		return
	}
	stream := s.stream
	done := s.streamDone
	s.l.Unlock()

	t.Logf("lock obtained on server.EmitRand(%t)\n", closeStream)

	if closeStream {
		t.Log("MOCK SERVER: closing stream upon request")
		close(done)
		return
	}

	if err := stream.Context().Err(); err != nil {
		t.Logf("MOCK SERVER: context error: %s\n", err)
		done <- err
		return
	}
	resp, err := s.PublicRand(s.stream.Context(), &drand.PublicRandRequest{})
	if err != nil {
		t.Logf("MOCK SERVER: public rand err: %s\n", err)
		done <- err
		return
	}
	if err = stream.Send(resp); err != nil {
		t.Logf("MOCK SERVER: stream send error: %s\n", err)
		done <- err
		return
	}
	t.Log("MOCK SERVER: emit round done")
}

func testValid(d *Data) {
	pub := d.Public
	pubPoint := key.KeyGroup.Point()
	if err := pubPoint.UnmarshalBinary(pub); err != nil {
		panic(err)
	}
	sig := decodeHex(d.Signature)

	var msg, invMsg []byte
	if !d.Scheme.DecouplePrevSig {
		prev := decodeHex(d.PreviousSignature)
		msg = sha256Hash(append(prev[:], roundToBytes(d.Round)...))
		invMsg = sha256Hash(append(prev[:], roundToBytes(d.Round-1)...))
	} else {
		msg = sha256Hash(roundToBytes(d.Round))
		invMsg = sha256Hash(roundToBytes(d.Round - 1))
	}

	if err := key.Scheme.VerifyRecovered(pubPoint, msg, sig); err != nil {
		panic(err)
	}
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
	Scheme            scheme.Scheme
}

func generateMockData(sch scheme.Scheme) *Data {
	secret := key.KeyGroup.Scalar().Pick(random.New())
	public := key.KeyGroup.Point().Mul(secret, nil)
	var previous [32]byte
	if _, err := rand.Reader.Read(previous[:]); err != nil {
		panic(err)
	}
	round := 1969
	prevRound := uint64(1968)

	var msg []byte
	if !sch.DecouplePrevSig {
		msg = sha256Hash(append(previous[:], roundToBytes(round)...))
	} else {
		msg = sha256Hash(roundToBytes(round))
	}

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
		Scheme:            sch,
	}
	return d
}

// nextMockData generates a valid Data for the next round when given the current round data.
func nextMockData(d *Data) *Data {
	previous := decodeHex(d.PreviousSignature)

	var msg []byte
	if !d.Scheme.DecouplePrevSig {
		msg = sha256Hash(append(previous[:], roundToBytes(d.Round+1)...))
	} else {
		msg = sha256Hash(roundToBytes(d.Round + 1))
	}

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
		Scheme:            d.Scheme,
	}
}

// NewMockGRPCPublicServer creates a listener that provides valid single-node randomness.
func NewMockGRPCPublicServer(bind string, badSecondRound bool, sch scheme.Scheme) (net.Listener, net.Service) {
	d := generateMockData(sch)
	testValid(d)

	d.BadSecondRound = badSecondRound
	d.Scheme = sch

	server := newMockServer(d)
	listener, err := net.NewGRPCListenerForPrivate(context.Background(), bind, "", "", server, true)
	if err != nil {
		panic(err)
	}
	server.addr = listener.Addr()
	return listener, server
}

// NewMockServer creates a server interface not bound to a newtork port
func NewMockServer(badSecondRound bool, sch scheme.Scheme) net.Service {
	d := generateMockData(sch)
	testValid(d)

	d.BadSecondRound = badSecondRound
	d.Scheme = sch

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
	err := binary.Write(&buff, binary.BigEndian, uint64(r))
	if err != nil {
		return nil
	}
	return buff.Bytes()
}

// NewMockBeacon provides a random beacon and the chain it validates against
func NewMockBeacon(sch scheme.Scheme) (*drand.ChainInfoPacket, *drand.PublicRandResponse) {
	d := generateMockData(sch)
	s := newMockServer(d)
	c, _ := s.ChainInfo(context.Background(), nil)
	r, _ := s.PublicRand(context.Background(), &drand.PublicRandRequest{Round: 1})

	return c, r
}
