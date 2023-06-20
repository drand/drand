package crypto

import (
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"os"

	// The package github.com/drand/kyber/sign/bls is deprecated because it is vulnerable to
	// rogue public-key attack against BLS aggregated signature. The new version of the protocol can be used to
	// make sure a signature aggregate cannot be verified by a forged key. You can find the protocol in kyber/sign/bdn.
	// Note that only the aggregation is broken by the attack and a later version will merge bls and asmbls.
	// The way we are using this package does not do any aggregation and we're only using simple signatures and thus
	// this is not a security issue for drand.
	//nolint:staticcheck
	"golang.org/x/crypto/blake2b"

	"github.com/drand/kyber"
	bls "github.com/drand/kyber-bls12381"
	"github.com/drand/kyber/pairing"
	"github.com/drand/kyber/sign"
	signBls "github.com/drand/kyber/sign/bls"
	"github.com/drand/kyber/sign/schnorr"
	"github.com/drand/kyber/sign/tbls"
	"github.com/drand/kyber/util/random"
)

type hashableBeacon interface {
	GetPreviousSignature() []byte
	GetRound() uint64
}

type signedBeacon interface {
	hashableBeacon
	GetSignature() []byte
}

// Scheme represents the cryptographic schemes supported by drand. It currently assumes the usage of pairings and
// it is important that the SigGroup and KeyGroup are properly set with respect to the ThresholdScheme, the AuthScheme
// also needs to be compatible with the KeyGroup, since it will use it to self-sign its own public key.
//
// Note: Scheme is not meant to be marshaled directly. Instead use the SchemeFromName
type Scheme struct {
	// The name of the scheme
	Name string
	// SigGroup is the group used to create the signatures; it must always be
	// different from the KeyGroup: G1 key group and G2 sig group or G1 sig group and G2 keygroup.
	SigGroup kyber.Group
	// KeyGroup is the group used to create the keys
	KeyGroup kyber.Group
	// ThresholdScheme is the signature scheme used, defining over which curve the signature
	// and keys respectively are.
	ThresholdScheme sign.ThresholdScheme
	// AuthScheme is the signature scheme used to identify public identities
	AuthScheme sign.Scheme
	// DKGAuthScheme is the signature scheme used to authenticate packets during broadcast in a DKG
	DKGAuthScheme sign.Scheme
	// Pairing is the underlying pairing Suite.
	Pairing pairing.Suite
	// the hash function used by this scheme
	IdentityHash func() hash.Hash `toml:"-"`
	// the DigestBeacon is used to generate the bytes that are getting signed
	DigestBeacon func(hashableBeacon) []byte `toml:"-"`
}

// VerifyBeacon is verifying the aggregated beacon against the provided group public key
func (s *Scheme) VerifyBeacon(b signedBeacon, pubkey kyber.Point) error {
	return s.ThresholdScheme.VerifyRecovered(pubkey, s.DigestBeacon(b), b.GetSignature())
}

func (s *Scheme) String() string {
	if s != nil {
		return s.Name
	}
	return ""
}

type schnorrSuite struct {
	kyber.Group
}

func (s *schnorrSuite) RandomStream() cipher.Stream {
	return random.New()
}

// DefaultSchemeID is the default scheme ID.
const DefaultSchemeID = "pedersen-bls-chained"

// NewPedersenBLSChained instantiate a scheme of type "pedersen-bls-chained" which is the original sheme used by drand
// since 2018. It links all beacons with the previous ones by "chaining" the signatures with the previous signature,
// preventing one to predict a future message that would be signed by the network before the previous signature is
// available. This however means this scheme is not compatible with "timelock encryption" as done by tlock.
// This schemes has the group public key on G1, so 48 bytes, and the beacon signatures on G2, so 96 bytes.
func NewPedersenBLSChained() (cs *Scheme) {
	var Pairing = bls.NewBLS12381Suite()
	var KeyGroup = Pairing.G1()
	var SigGroup = Pairing.G2()
	var ThresholdScheme = tbls.NewThresholdSchemeOnG2(Pairing)
	var AuthScheme = signBls.NewSchemeOnG2(Pairing)
	var DKGAuthScheme = schnorr.NewScheme(&schnorrSuite{KeyGroup})
	var IdentityHashFunc = func() hash.Hash { h, _ := blake2b.New256(nil); return h }
	// Chained means we're hashing the previous signature and the round number to make it an actual "chain"
	var DigestFunc = func(b hashableBeacon) []byte {
		h := sha256.New()

		if len(b.GetPreviousSignature()) > 0 {
			_, _ = h.Write(b.GetPreviousSignature())
		}
		_ = binary.Write(h, binary.BigEndian, b.GetRound())
		return h.Sum(nil)
	}

	return &Scheme{
		Name:            DefaultSchemeID,
		SigGroup:        SigGroup,
		KeyGroup:        KeyGroup,
		ThresholdScheme: ThresholdScheme,
		AuthScheme:      AuthScheme,
		DKGAuthScheme:   DKGAuthScheme,
		Pairing:         Pairing,
		IdentityHash:    IdentityHashFunc,
		DigestBeacon:    DigestFunc,
	}
}

// UnchainedSchemeID is the scheme id used to set unchained randomness on beacons.
const UnchainedSchemeID = "pedersen-bls-unchained"

// NewPedersenBLSUnchained instantiate a scheme of type "pedersen-bls-unchained" which removes the link of  all beacons
// with the previous ones by only hashing the round number as the message being signed. This scheme is compatible with
// "timelock encryption" as done by tlock.
// This schemes has the group public key on G1, so 48 bytes, and the beacon signatures on G2, so 96 bytes.
//
//nolint:dupl
func NewPedersenBLSUnchained() (cs *Scheme) {
	var Pairing = bls.NewBLS12381Suite()
	var KeyGroup = Pairing.G1()
	var SigGroup = Pairing.G2()
	var ThresholdScheme = tbls.NewThresholdSchemeOnG2(Pairing)
	var AuthScheme = signBls.NewSchemeOnG2(Pairing)
	var DKGAuthScheme = schnorr.NewScheme(&schnorrSuite{KeyGroup})
	var IdentityHashFunc = func() hash.Hash { h, _ := blake2b.New256(nil); return h }
	// Unchained means we're only hashing the round number
	var DigestFunc = func(b hashableBeacon) []byte {
		h := sha256.New()
		_ = binary.Write(h, binary.BigEndian, b.GetRound())
		return h.Sum(nil)
	}

	return &Scheme{
		Name:            UnchainedSchemeID,
		SigGroup:        SigGroup,
		KeyGroup:        KeyGroup,
		ThresholdScheme: ThresholdScheme,
		AuthScheme:      AuthScheme,
		DKGAuthScheme:   DKGAuthScheme,
		Pairing:         Pairing,
		IdentityHash:    IdentityHashFunc,
		DigestBeacon:    DigestFunc,
	}
}

// ShortSigSchemeID is the scheme id used to set unchained randomness on beacons with G1 and G2 swapped.
const ShortSigSchemeID = "bls-unchained-on-g1"

// NewPedersenBLSUnchainedSwapped instantiate a scheme of type "bls-unchained-on-g1" which is also unchained, only
// hashing the round number as the message being signed in beacons. This scheme is also compatible with
// "timelock encryption" as done by tlock.
// This schemes has the group public key on G2, so 96 bytes, and the beacon signatures on G1, so 48 bytes.
// This means databases of beacons produced with this scheme are almost half the size of the other schemes.
//
//nolint:dupl
func NewPedersenBLSUnchainedSwapped() (cs *Scheme) {
	var Pairing = bls.NewBLS12381Suite()
	var KeyGroup = Pairing.G2()
	var SigGroup = Pairing.G1()
	// using G1 for the ThresholdScheme since it allows beacons to have shorter signatures, reducing the size of any
	// database storing all existing beacons by half compared to using G2.
	var ThresholdScheme = tbls.NewThresholdSchemeOnG1(Pairing)
	var AuthScheme = signBls.NewSchemeOnG1(Pairing)
	var DKGAuthScheme = schnorr.NewScheme(&schnorrSuite{KeyGroup})
	var IdentityHashFunc = func() hash.Hash { h, _ := blake2b.New256(nil); return h }
	// Unchained means we're only hashing the round number
	var DigestFunc = func(b hashableBeacon) []byte {
		h := sha256.New()
		_ = binary.Write(h, binary.BigEndian, b.GetRound())
		return h.Sum(nil)
	}

	return &Scheme{
		Name:            ShortSigSchemeID,
		SigGroup:        SigGroup,
		KeyGroup:        KeyGroup,
		ThresholdScheme: ThresholdScheme,
		AuthScheme:      AuthScheme,
		DKGAuthScheme:   DKGAuthScheme,
		Pairing:         Pairing,
		IdentityHash:    IdentityHashFunc,
		DigestBeacon:    DigestFunc,
	}
}

func SchemeFromName(schemeName string) (*Scheme, error) {
	switch schemeName {
	case DefaultSchemeID:
		return NewPedersenBLSChained(), nil
	case UnchainedSchemeID:
		return NewPedersenBLSUnchained(), nil
	case ShortSigSchemeID:
		return NewPedersenBLSUnchainedSwapped(), nil
	default:
		return nil, fmt.Errorf("invalid scheme name '%s'", schemeName)
	}
}

var schemeIDs = []string{DefaultSchemeID, UnchainedSchemeID, ShortSigSchemeID}

// ListSchemes will return a slice of valid scheme ids
func ListSchemes() []string {
	return schemeIDs
}

// GetSchemeByIDWithDefault allows the user to retrieve the scheme configuration looking by its ID. It will return a boolean which indicates
// if the scheme was found or not. In addition to it, if the received ID is an empty string,
// it will return the default defined scheme
func GetSchemeByIDWithDefault(id string) (*Scheme, error) {
	if id == "" {
		id = DefaultSchemeID
	}

	return SchemeFromName(id)
}
func GetSchemeByID(id string) (*Scheme, bool) {
	if id == "" {
		id = DefaultSchemeID
	}

	sch, err := SchemeFromName(id)
	if err != nil {
		return nil, false
	}
	return sch, true
}

// GetSchemeFromEnv allows the user to retrieve the scheme configuration looking by the ID set on an
// environmental variable. If the scheme is not found, function will panic.
func GetSchemeFromEnv() (*Scheme, error) {
	id := os.Getenv("SCHEME_ID")

	return GetSchemeByIDWithDefault(id)
}

// RandomnessFromSignature derives the round randomness from its signature. We are using sha256 currently
// but it could use blake2b instead or another hash. Hashing the signature is important because the algebraic structure
// of the elliptic curve points that correspond to signatures does not map uniformly with all possible bit string, but
// a signature is indistinguishable from any random point on that elliptic curve.
func RandomnessFromSignature(sig []byte) []byte {
	out := sha256.Sum256(sig)
	return out[:]
}
