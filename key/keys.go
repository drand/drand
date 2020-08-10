package key

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"net"

	kyber "github.com/drand/kyber"
	"github.com/drand/kyber/share"
	dkg "github.com/drand/kyber/share/dkg"
	"github.com/drand/kyber/util/random"

	proto "github.com/drand/drand/protobuf/drand"
)

// Pair is a wrapper around a random scalar  and the corresponding public
// key
type Pair struct {
	Key    kyber.Scalar
	Public *Identity
}

// Identity holds the corresponding public key of a Private. It also includes a
// valid internet facing ipv4 address where to this reach the node holding the
// public / private key pair.
type Identity struct {
	Key       kyber.Point
	Addr      string
	TLS       bool
	Signature []byte
}

// Address implements the net.Peer interface
func (i *Identity) Address() string {
	return i.Addr
}

// IsTLS returns true if this address is reachable over TLS.
func (i *Identity) IsTLS() bool {
	return i.TLS
}

func (i *Identity) String() string {
	return fmt.Sprintf("{%s - %s}", i.Address(), i.Key.String())
}

// Hash returns the hash of the public key without signing the signature. The hash
// is the input to the signature scheme. It does _not_ hash the address & tls
// field as those may need to change while the node keeps the same key.
func (i *Identity) Hash() []byte {
	h := hashFunc()
	_, _ = i.Key.MarshalTo(h)
	return h.Sum(nil)
}

// ValidSignature returns true if the signature included in this identity is
// correct or not
func (i *Identity) ValidSignature() error {
	msg := i.Hash()
	return AuthScheme.Verify(i.Key, msg, i.Signature)
}

// Equal indicates if two identities are equal
func (i *Identity) Equal(i2 *Identity) bool {
	if i.Addr != i2.Addr {
		return false
	}
	if i.TLS != i2.TLS {
		return false
	}
	if !i.Key.Equal(i2.Key) {
		return false
	}
	return true
}

// SelfSign signs the public key with the key pair
func (p *Pair) SelfSign() {
	msg := p.Public.Hash()
	signature, _ := AuthScheme.Sign(p.Key, msg)
	p.Public.Signature = signature
}

// NewKeyPair returns a freshly created private / public key pair. The group is
// decided by the group variable by default.
func NewKeyPair(address string) *Pair {
	key := KeyGroup.Scalar().Pick(random.New())
	pubKey := KeyGroup.Point().Mul(key, nil)
	pub := &Identity{
		Key:  pubKey,
		Addr: address,
	}
	p := &Pair{
		Key:    key,
		Public: pub,
	}
	p.SelfSign()
	return p
}

// NewTLSKeyPair returns a fresh keypair associated with the given address
// reachable over TLS.
func NewTLSKeyPair(address string) *Pair {
	kp := NewKeyPair(address)
	kp.Public.TLS = true
	kp.SelfSign()
	return kp
}

// PairTOML is the TOML-able version of a private key
type PairTOML struct {
	Key string
}

// PublicTOML is the TOML-able version of a public key
type PublicTOML struct {
	Address   string
	Key       string
	TLS       bool
	Signature string
}

// TOML returns a struct that can be marshaled using a TOML-encoding library
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

	var err error
	p.Key, err = StringToScalar(KeyGroup, ptoml.Key)
	p.Public = new(Identity)
	return err
}

// TOMLValue returns an empty TOML-compatible interface value
func (p *Pair) TOMLValue() interface{} {
	return &PairTOML{}
}

// FromTOML loads reads the TOML description of the public key
func (i *Identity) FromTOML(t interface{}) error {
	ptoml, ok := t.(*PublicTOML)
	if !ok {
		return errors.New("public can't decode from non PublicTOML struct")
	}
	var err error
	i.Key, err = StringToPoint(KeyGroup, ptoml.Key)
	if err != nil {
		return fmt.Errorf("decoding public key: %s", err)
	}
	i.Addr = ptoml.Address
	i.TLS = ptoml.TLS
	if ptoml.Signature != "" {
		i.Signature, err = hex.DecodeString(ptoml.Signature)
	}
	return err
}

// TOML returns a empty TOML-compatible version of the public key
func (i *Identity) TOML() interface{} {
	hexKey := PointToString(i.Key)
	return &PublicTOML{
		Address:   i.Addr,
		Key:       hexKey,
		TLS:       i.TLS,
		Signature: hex.EncodeToString(i.Signature),
	}
}

// TOMLValue returns a TOML-compatible interface value
func (i *Identity) TOMLValue() interface{} {
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

// IdentityFromProto creates an identity from its wire representation and
// verifies it validity.
func IdentityFromProto(n *proto.Identity) (*Identity, error) {
	_, _, err := net.SplitHostPort(n.GetAddress())
	if err != nil {
		return nil, err
	}
	public := KeyGroup.Point()
	if err := public.UnmarshalBinary(n.GetKey()); err != nil {
		return nil, err
	}

	id := &Identity{
		Addr:      n.GetAddress(),
		TLS:       n.Tls,
		Key:       public,
		Signature: n.GetSignature(),
	}
	return id, nil
}

// ToProto marshals an identity into protobuf format
func (i *Identity) ToProto() *proto.Identity {
	buff, _ := i.Key.MarshalBinary()
	return &proto.Identity{
		Address:   i.Addr,
		Key:       buff,
		Tls:       i.TLS,
		Signature: i.Signature,
	}
}

// Share represents the private information that a node holds after a successful
// DKG. This information MUST stay private !
type Share dkg.DistKeyShare

// PubPoly returns the public polynomial that can be used to verify any
// individual patial signature
func (s *Share) PubPoly() *share.PubPoly {
	return share.NewPubPoly(KeyGroup, KeyGroup.Point().Base(), s.Commits)
}

// PrivateShare returns the private share used to produce a partial signature
func (s *Share) PrivateShare() *share.PriShare {
	return s.Share
}

// Public returns the distributed public key associated with the distributed key
// share
func (s *Share) Public() *DistPublic {
	return &DistPublic{s.Commits}
}

// TOML returns a TOML-compatible version of this share
func (s *Share) TOML() interface{} {
	dtoml := &ShareTOML{}
	dtoml.Commits = make([]string, len(s.Commits))
	for i, c := range s.Commits {
		dtoml.Commits[i] = PointToString(c)
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
		p, err := StringToPoint(KeyGroup, c)
		if err != nil {
			return fmt.Errorf("share.Commit[%d] corruputed: %s", i, err)
		}
		s.Commits[i] = p
	}

	sshare, err := StringToScalar(KeyGroup, t.Share)
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

// PubPoly provides the public polynomial commitment
func (d *DistPublic) PubPoly() *share.PubPoly {
	return share.NewPubPoly(KeyGroup, KeyGroup.Point().Base(), d.Coefficients)
}

// Key returns the first coefficient as representing the public key to be used
// to verify signatures issued by the distributed key.
func (d *DistPublic) Key() kyber.Point {
	return d.Coefficients[0]
}

// Hash computes the hash of this distributed key.
func (d *DistPublic) Hash() []byte {
	h := hashFunc()
	for _, c := range d.Coefficients {
		buff, _ := c.MarshalBinary()
		_, _ = h.Write(buff)
	}
	return h.Sum(nil)
}

// DistPublicTOML is a TOML compatible value of a DistPublic
type DistPublicTOML struct {
	Coefficients []string
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
		points[i], err = StringToPoint(KeyGroup, s)
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

// Equal returns if all coefficients of the public key d are equal to those of
// d2
func (d *DistPublic) Equal(d2 *DistPublic) bool {
	if len(d.Coefficients) != len(d2.Coefficients) {
		return false
	}
	for i := range d.Coefficients {
		p1 := d.Coefficients[i]
		p2 := d2.Coefficients[i]
		if !p1.Equal(p2) {
			return false
		}
	}
	return true
}

// DefaultThreshold return floor(n / 2) + 1
func DefaultThreshold(n int) int {
	return MinimumT(n)
}
