package key

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"

	kyber "github.com/dedis/kyber"
	"github.com/dedis/kyber/pairing/bn256"
	"github.com/dedis/kyber/share"
	"github.com/dedis/kyber/share/dkg/pedersen"
	"github.com/dedis/kyber/util/random"
)

var Pairing = bn256.NewSuite()
var G1 = Pairing.G1()
var G2 = Pairing.G2()

// Pair is a wrapper around a random scalar  and the corresponding public
// key in G2
type Pair struct {
	Key    kyber.Scalar
	Public *Identity
}

// Identity holds the corresponding public key of a Private. It also includes a
// valid internet facing ipv4 address where to this reach the node holding the
// public / private key pair.
type Identity struct {
	Key  kyber.Point
	Addr string
	TLS  bool
}

// Address implements the net.Peer interface
func (i *Identity) Address() string {
	return i.Addr
}

func (i *Identity) IsTLS() bool {
	return i.TLS
}

// NewKeyPair returns a freshly created private / public key pair. The group is
// decided by the group variable by default. Currently, drand only supports
// bn256.
func NewKeyPair(address string) *Pair {
	key := G2.Scalar().Pick(random.New())
	pubKey := G2.Point().Mul(key, nil)
	pub := &Identity{
		Key:  pubKey,
		Addr: address,
	}
	return &Pair{
		Key:    key,
		Public: pub,
	}
}

func NewTLSKeyPair(address string) *Pair {
	kp := NewKeyPair(address)
	kp.Public.TLS = true
	return kp
}

// PairTOML is the TOML-able version of a private key
type PairTOML struct {
	Key string
}

// PublicTOML is the TOML-able version of a public key
type PublicTOML struct {
	Address string
	Key     string
	TLS     bool
}

// TOML returns a struct that can be marshalled using a TOML-encoding library
func (p *Pair) TOML() interface{} {
	hexKey := ScalarToString(p.Key)
	return &PairTOML{hexKey}
}

// FromTOML constructs the private key from an unmarshalled structure from TOML
func (p *Pair) FromTOML(i interface{}) error {
	ptoml, ok := i.(*PairTOML)
	if !ok {
		return errors.New("private can't decode toml from non PairTOML struct")
	}

	buff, err := hex.DecodeString(ptoml.Key)
	if err != nil {
		return err
	}
	p.Key = G2.Scalar()
	if err := p.Key.UnmarshalBinary(buff); err != nil {
		return err
	}
	p.Public = new(Identity)
	return nil
}

// TOMLValue returns an empty TOML-compatible interface value
func (p *Pair) TOMLValue() interface{} {
	return &PairTOML{}
}

// Equal returns true if the cryptographic public key of p equals p2's
func (p *Identity) Equal(p2 *Identity) bool {
	return p.Key.Equal(p2.Key)
}

// FromTOML loads reads the TOML description of the public key
func (p *Identity) FromTOML(i interface{}) error {
	ptoml, ok := i.(*PublicTOML)
	if !ok {
		return errors.New("Public can't decode from non PublicTOML struct")
	}
	buff, err := hex.DecodeString(ptoml.Key)
	if err != nil {
		return err
	}
	p.Addr = ptoml.Address
	p.Key = G2.Point()
	p.TLS = ptoml.TLS
	return p.Key.UnmarshalBinary(buff)
}

// TOML returns a empty TOML-compatible version of the public key
func (p *Identity) TOML() interface{} {
	hex := PointToString(p.Key)
	return &PublicTOML{
		Address: p.Addr,
		Key:     hex,
		TLS:     p.TLS,
	}
}

// TOMLValue returns a TOML-compatible interface value
func (p *Identity) TOMLValue() interface{} {
	return &PublicTOML{}
}

// ByKey is simply an interface to sort lexig
type ByKey []*Identity

func (b ByKey) Len() int {
	return len(b)
}

func (b ByKey) Swap(i, j int) {
	(b)[i], (b)[j] = (b)[j], (b)[i]
}

func (b ByKey) Less(i, j int) bool {
	is, _ := (b)[i].Key.MarshalBinary()
	js, _ := (b)[j].Key.MarshalBinary()
	return bytes.Compare(is, js) < 0
}

// Share represents the private information that a node holds after a successful
// DKG. This information MUST stay private !
type Share dkg.DistKeyShare

// Public returns the distributed public key associated with the distributed key
// share
func (s *Share) Public() *DistPublic {
	return &DistPublic{s.Commits}
}

// TOML returns a TOML-compatible version of this share
func (s *Share) TOML() interface{} {
	dtoml := &ShareTOML{}
	dtoml.Commits = make([]string, len(s.Commits))
	dtoml.PrivatePoly = make([]string, len(s.PrivatePoly))
	for i, c := range s.Commits {
		dtoml.Commits[i] = PointToString(c)
	}
	for i, c := range s.PrivatePoly {
		dtoml.PrivatePoly[i] = ScalarToString(c)
	}
	dtoml.Share = ScalarToString(s.Share.V)
	dtoml.Index = s.Share.I
	return dtoml
}

// FromTOML initializes the share from the given TOML-compatible share interface
func (s *Share) FromTOML(i interface{}) error {
	t, ok := i.(*ShareTOML)
	if !ok {
		return errors.New("invalid struct received for share")
	}
	s.Commits = make([]kyber.Point, len(t.Commits))
	for i, c := range t.Commits {
		p, err := StringToPoint(G2, c)
		if err != nil {
			return fmt.Errorf("share.Commit[%d] corruputed: %s", i, err)
		}
		s.Commits[i] = p
	}

	s.PrivatePoly = make([]kyber.Scalar, len(t.PrivatePoly))
	for i, c := range t.PrivatePoly {
		coeff, err := StringToScalar(G2, c)
		if err != nil {
			return fmt.Errorf("share.PrivatePoly[%d] corrupted: %s", i, err)
		}
		s.PrivatePoly[i] = coeff
	}
	sshare, err := StringToScalar(G2, t.Share)
	if err != nil {
		return fmt.Errorf("share.Share corrupted: %s", err)
	}
	s.Share = &share.PriShare{V: sshare, I: t.Index}
	return nil
}

// TOMLValue returns an empty TOML compatible interface of that Share
func (s *Share) TOMLValue() interface{} {
	return &ShareTOML{}
}

// ShareTOML is the TOML representation of a dkg.DistKeyShare
type ShareTOML struct {
	// index of the share.
	Index int
	// evaluation of the private polynomial.
	Share string
	// coefficients of the public polynomial.
	Commits []string
	// coefficients of the individual private polynomial generated by the node
	// at the given index.
	PrivatePoly []string
}

// DistPublic represents the distributed public key generated during a DKG. This
// is the information that can be safely exported to end users verifying a
// drand signature. It is the list of all commitments of the coefficients of the
// private distributed polynomial.
type DistPublic struct {
	Coefficients []kyber.Point
}

// DistPublicTOML is a TOML compatible value of a DistPublic
type DistPublicTOML struct {
	Coefficients []string
}

// Key returns the first coefficient as representing the public key to be used
// to verify signatures issued by the distributed key.
func (d *DistPublic) Key() kyber.Point {
	return d.Coefficients[0]
}

// TOML returns a TOML-compatible version of d
func (d *DistPublic) TOML() interface{} {
	strings := make([]string, len(d.Coefficients))
	for i, s := range d.Coefficients {
		strings[i] = PointToString(s)
	}
	return &DistPublicTOML{strings}
}

// FromTOML initializes d from the TOML-compatible version of a DistPublic
func (d *DistPublic) FromTOML(i interface{}) error {
	dtoml, ok := i.(*DistPublicTOML)
	if !ok {
		return errors.New("wrong interface: expected DistPublicTOML")
	}
	points := make([]kyber.Point, len(dtoml.Coefficients))
	var err error
	for i, s := range dtoml.Coefficients {
		points[i], err = StringToPoint(G2, s)
		if err != nil {
			return err
		}
	}
	d.Coefficients = points
	return nil
}

// TOMLValue returns an empty TOML-compatible dist public interface
func (d *DistPublic) TOMLValue() interface{} {
	return &DistPublicTOML{}
}

// BeaconSignature is the final reconstructed BLS signature that is saved in the
// filesystem.
type BeaconSignature struct {
	Timestamp   int64
	PreviousSig string
	Signature   string
}

// NewBeaconSignature initializes a beacon signature from
// - a timestamp
// - a previous sig. Can be nil if there is no previous signature
// - a signature of the timestamp and the previous sig
func NewBeaconSignature(timestamp int64, previousSig, signature []byte) *BeaconSignature {
	hexSig := hex.EncodeToString(signature)
	hexPrev := hex.EncodeToString(previousSig)
	return &BeaconSignature{
		Timestamp:   timestamp,
		PreviousSig: hexPrev,
		Signature:   hexSig,
	}
}

// TOML returns a TOML-compatible version of this beacon signature
func (b *BeaconSignature) TOML() interface{} {
	return b
}

// FromTOML initializes b from a TOML-compatible version of a beacon signature
func (b *BeaconSignature) FromTOML(i interface{}) error {
	bb, ok := i.(*BeaconSignature)
	if !ok {
		return errors.New("beacon signature can't decode: wrong type")
	}
	*b = *bb
	return nil
}

// TOMLValue returns an empty TOML-compatible version of a beacon signature
func (b *BeaconSignature) TOMLValue() interface{} {
	return &BeaconSignature{}
}

// RawSig returns the signature
func (b *BeaconSignature) RawSig() []byte {
	s, err := base64.StdEncoding.DecodeString(b.Signature)
	if err != nil {
		panic("beacon signature have invalid base64 encoded ! File corrupted ? Attack ? God ? Pesto ?")
	}
	return s
}

// PointToString returns a hex-encoded string representation of the given point.
func PointToString(p kyber.Point) string {
	buff, _ := p.MarshalBinary()
	return hex.EncodeToString(buff)
}

// ScalarToString returns a hex-encoded string representation of the given scalar.
func ScalarToString(s kyber.Scalar) string {
	buff, _ := s.MarshalBinary()
	return hex.EncodeToString(buff)
}

// StringToPoint unmarshals a point in the given group from the given string.
func StringToPoint(g kyber.Group, s string) (kyber.Point, error) {
	buff, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	p := g.Point()
	return p, p.UnmarshalBinary(buff)
}

// StringToScalar unmarshals a scalar in the given group from the given string.
func StringToScalar(g kyber.Group, s string) (kyber.Scalar, error) {
	buff, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	sc := g.Scalar()
	return sc, sc.UnmarshalBinary(buff)
}

func DefaultThreshold(n int) int {
	return (n*2)/3 + 1
}
