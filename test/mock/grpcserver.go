package mock

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	clock "github.com/jonboulle/clockwork"

	"github.com/drand/drand/crypto"
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

type logger interface {
	Log(args ...any)
}

type fmtLogger struct{}

func (fmtLogger) Log(params ...any) {
	fmt.Println(params...)
}

// Server fake
type Server struct {
	addr string
	*testnet.EmptyServer
	l          sync.Mutex
	stream     drand.Public_PublicRandStreamServer
	streamDone chan error
	d          *Data
	t          logger
	clk        clock.Clock
	chainInfo  *drand.ChainInfoPacket
}

func newMockServer(t logger, d *Data, clk clock.Clock) *Server {
	if t == nil {
		t = fmtLogger{}
	}

	return &Server{
		EmptyServer: new(testnet.EmptyServer),
		d:           d,
		t:           t,
		clk:         clk,
		chainInfo: &drand.ChainInfoPacket{
			Period:      uint32(d.Period.Seconds()),
			GenesisTime: d.Genesis,
			PublicKey:   d.Public,
			SchemeID:    d.Scheme.Name,
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
	streamDone := make(chan error, 1)
	s.l.Lock()
	s.streamDone = streamDone
	s.stream = stream
	s.l.Unlock()

	// We want to remove the stream here but not while it's in use.
	// To fix this, we'll defer setting stream to nil and wait for
	// the launched operations to finish, see below.
	defer func() {
		s.l.Lock()
		s.stream = nil
		s.l.Unlock()
	}()

	// Wait for values to be sent before returning from this function.
	return <-streamDone
}

// EmitRand will cause the next round to be emitted by a previous call to `PublicRandomStream`
func (s *Server) EmitRand(closeStream bool) {
	s.l.Lock()
	if s.stream == nil {
		s.t.Log("MOCK SERVER: stream nil")
		s.l.Unlock()
		return
	}
	stream := s.stream
	done := s.streamDone
	s.l.Unlock()

	if closeStream {
		close(done)
		s.t.Log("MOCK SERVER: closing stream upon request")
		return
	}

	if err := stream.Context().Err(); err != nil {
		done <- err
		s.t.Log("MOCK SERVER: context error ", err)
		return
	}
	s.clk.(clock.FakeClock).Advance(time.Duration(s.chainInfo.Period) * time.Second)
	resp, err := s.PublicRand(s.stream.Context(), &drand.PublicRandRequest{})
	if err != nil {
		done <- err
		s.t.Log("MOCK SERVER: public rand err:", err)
		return
	}
	s.t.Log(fmt.Sprintf("MOCK SERVER: sending round: %d time.Now: %d", resp.Round, s.clk.Now().Unix()))
	if err = stream.Send(resp); err != nil {
		done <- err
		s.t.Log("MOCK SERVER: stream send error:", err)
		return
	}
	s.t.Log("MOCK SERVER: emit round done", resp.Round)
}

func testValid(d *Data) {
	pub := d.Public
	pubPoint := d.Scheme.KeyGroup.Point()
	if err := pubPoint.UnmarshalBinary(pub); err != nil {
		panic(err)
	}
	sig := decodeHex(d.Signature)

	var msg, invMsg []byte
	if d.Scheme.Name == crypto.DefaultSchemeID { // we're in chained mode
		prev := decodeHex(d.PreviousSignature)
		msg = sha256Hash(append(prev[:], roundToBytes(d.Round)...))
		invMsg = sha256Hash(append(prev[:], roundToBytes(d.Round-1)...))
	} else { // we are in unchained mode
		msg = sha256Hash(roundToBytes(d.Round))
		invMsg = sha256Hash(roundToBytes(d.Round - 1))
	}

	if err := d.Scheme.ThresholdScheme.VerifyRecovered(pubPoint, msg, sig); err != nil {
		panic(err)
	}
	if err := d.Scheme.ThresholdScheme.VerifyRecovered(pubPoint, invMsg, sig); err == nil {
		panic("should be invalid signature")
	}
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
	Scheme            *crypto.Scheme
}

func generateMockData(sch *crypto.Scheme, clk clock.Clock) *Data {
	secret := sch.KeyGroup.Scalar().Pick(random.New())
	public := sch.KeyGroup.Point().Mul(secret, nil)
	var previous [32]byte
	if _, err := rand.Reader.Read(previous[:]); err != nil {
		panic(err)
	}
	round := 1969
	prevRound := uint64(1968)

	var msg []byte
	if sch.Name == crypto.DefaultSchemeID { // we're in chained mode
		msg = sha256Hash(append(previous[:], roundToBytes(round)...))
	} else { // we're in unchained mode
		msg = sha256Hash(roundToBytes(round))
	}

	sshare := share.PriShare{I: 0, V: secret}
	tsig, err := sch.ThresholdScheme.Sign(&sshare, msg)
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
		Genesis:           clk.Now().Add(period * 1969 * -1).Unix(),
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
	if d.Scheme.Name == crypto.DefaultSchemeID { // we're in chained mode
		msg = sha256Hash(append(previous[:], roundToBytes(d.Round+1)...))
	} else { // we're in unchained mode
		msg = sha256Hash(roundToBytes(d.Round + 1))
	}

	sshare := share.PriShare{I: 0, V: d.secret}
	tsig, err := d.Scheme.ThresholdScheme.Sign(&sshare, msg)
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
func NewMockGRPCPublicServer(t *testing.T, bind string, badSecondRound bool, sch *crypto.Scheme, clk clock.Clock) (net.Listener, net.Service) {
	d := generateMockData(sch, clk)
	testValid(d)

	d.BadSecondRound = badSecondRound
	d.Scheme = sch

	server := newMockServer(t, d, clk)
	listener, err := net.NewGRPCListenerForPrivate(context.Background(), bind, "", "", server, true)
	if err != nil {
		panic(err)
	}
	server.addr = listener.Addr()
	return listener, server
}

// NewMockServer creates a server interface not bound to a newtork port
func NewMockServer(t *testing.T, badSecondRound bool, sch *crypto.Scheme, clk clock.Clock) net.Service {
	d := generateMockData(sch, clk)
	testValid(d)

	d.BadSecondRound = badSecondRound
	d.Scheme = sch

	server := newMockServer(t, d, clk)
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
func NewMockBeacon(t *testing.T, sch *crypto.Scheme, clk clock.Clock) (*drand.ChainInfoPacket, *drand.PublicRandResponse) {
	d := generateMockData(sch, clk)
	s := newMockServer(t, d, clk)
	c, _ := s.ChainInfo(context.Background(), nil)
	r, _ := s.PublicRand(context.Background(), &drand.PublicRandRequest{Round: 1})

	return c, r
}

func (s *Server) Propose(_ context.Context, _ *drand.ProposalTerms) (*drand.EmptyResponse, error) {
	return nil, errors.New("unimplemented for mock server")
}

func (s *Server) Accept(_ context.Context, _ *drand.AcceptProposal) (*drand.EmptyResponse, error) {
	return nil, errors.New("unimplemented for mock server")
}

func (s *Server) Reject(_ context.Context, _ *drand.RejectProposal) (*drand.EmptyResponse, error) {
	return nil, errors.New("unimplemented for mock server")
}

func (s *Server) Abort(_ context.Context, _ *drand.AbortDKG) (*drand.EmptyResponse, error) {
	return nil, errors.New("unimplemented for mock server")
}

func (s *Server) Execute(_ context.Context, _ *drand.StartExecution) (*drand.EmptyResponse, error) {
	return nil, errors.New("unimplemented for mock server")
}

func (s *Server) Join(ctx context.Context, options *drand.JoinOptions) (*drand.EmptyResponse, error) {
	return nil, errors.New("unimplemented for mock server")
}

func (s *Server) StartNetwork(ctx context.Context, in *drand.FirstProposalOptions) (*drand.EmptyResponse, error) {
	return nil, errors.New("unimplemented for mock server")

}

func (s *Server) StartProposal(ctx context.Context, in *drand.ProposalOptions) (*drand.EmptyResponse, error) {
	return nil, errors.New("unimplemented for mock server")
}

func (s *Server) StartAbort(ctx context.Context, in *drand.AbortOptions) (*drand.EmptyResponse, error) {
	return nil, errors.New("unimplemented for mock server")
}

func (s *Server) StartExecute(ctx context.Context, in *drand.ExecutionOptions) (*drand.EmptyResponse, error) {
	return nil, errors.New("unimplemented for mock server")
}

func (s *Server) StartAccept(ctx context.Context, in *drand.AcceptOptions) (*drand.EmptyResponse, error) {
	return nil, errors.New("unimplemented for mock server")
}

func (s *Server) StartReject(ctx context.Context, in *drand.RejectOptions) (*drand.EmptyResponse, error) {
	return nil, errors.New("unimplemented for mock server")
}

func (s *Server) DKGStatus(ctx context.Context, in *drand.DKGStatusRequest) (*drand.DKGStatusResponse, error) {
	return nil, errors.New("unimplemented for mock server")
}
