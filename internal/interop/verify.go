// Package interop verifies historical drand beacons stored in a boltdb file
// against the go.dedis.ch/kyber/v4 dependency.
//
// drand historically depended on github.com/drand/kyber (a fork) together with
// github.com/drand/kyber-bls12381 (the kilic BLS12-381 backend). When migrating
// to the upstream go.dedis.ch/kyber/v4 module we must guarantee that every
// signature ever produced still verifies under the new dependency. kyber v4
// ships three independent BLS12-381 implementations (kilic, circl and gnark),
// so this package re-verifies each beacon with every backend that is able to
// reproduce the scheme, catching any divergence between implementations.
package interop

import (
	"context"
	"errors"
	"fmt"

	"go.dedis.ch/kyber/v4"
	"go.dedis.ch/kyber/v4/pairing"
	"go.dedis.ch/kyber/v4/pairing/bls12381/circl"
	"go.dedis.ch/kyber/v4/pairing/bls12381/gnark"
	"go.dedis.ch/kyber/v4/sign"
	"go.dedis.ch/kyber/v4/sign/tbls"

	"github.com/drand/drand/v2/common"
	chaininfo "github.com/drand/drand/v2/common/chain"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/chain"
	"github.com/drand/drand/v2/internal/chain/boltdb"
	chainerrors "github.com/drand/drand/v2/internal/chain/errors"
)

// BLS12381Backends lists the BLS12-381 implementations shipped by go.dedis.ch/kyber/v4.
//
//   - kilic  is the backend drand uses in production (github.com/kilic/bls12-381),
//     and the only one that supports custom hash-to-curve domain separation tags.
//   - circl  is Cloudflare's implementation (github.com/cloudflare/circl).
//   - gnark  is the Consensys implementation (github.com/consensys/gnark-crypto).
//
// circl and gnark hard-code the standard RFC9380 domain separation tags, so they
// can verify the spec-compliant drand schemes but cannot reproduce the deprecated
// "bls-unchained-on-g1" scheme, which abuses the G2 DST on G1.
var BLS12381Backends = []string{"kilic", "circl", "gnark"}

// ErrUnsupportedBackend is returned when a backend cannot reproduce a given scheme.
var ErrUnsupportedBackend = errors.New("backend cannot reproduce this scheme")

// Verifier verifies beacons of a single scheme using a single BLS12-381 backend.
type Verifier struct {
	backend   string
	scheme    *crypto.Scheme
	threshold sign.ThresholdScheme
	pub       kyber.Point
}

// NewVerifier builds a verifier for the given scheme name and backend, using the
// provided raw (compressed) public key bytes. It returns ErrUnsupportedBackend
// wrapped in the error if the backend cannot reproduce the scheme.
func NewVerifier(schemeName, backend string, pubKey []byte) (*Verifier, error) {
	sch, err := crypto.GetSchemeByID(schemeName)
	if err != nil {
		return nil, err
	}

	var keyGroup kyber.Group
	var threshold sign.ThresholdScheme

	switch backend {
	case "kilic":
		// This is the production code path: drand's schemes are built on kilic.
		keyGroup = sch.KeyGroup
		threshold = sch.ThresholdScheme
	case "circl", "gnark":
		suite, sigOnG1, err := bls12381Suite(backend, schemeName)
		if err != nil {
			return nil, err
		}
		if sigOnG1 {
			keyGroup = suite.G2()
			threshold = tbls.NewThresholdSchemeOnG1(suite)
		} else {
			keyGroup = suite.G1()
			threshold = tbls.NewThresholdSchemeOnG2(suite)
		}
	default:
		return nil, fmt.Errorf("unknown backend %q", backend)
	}

	pub := keyGroup.Point()
	if err := pub.UnmarshalBinary(pubKey); err != nil {
		return nil, fmt.Errorf("backend %s: unmarshalling public key: %w", backend, err)
	}

	return &Verifier{backend: backend, scheme: sch, threshold: threshold, pub: pub}, nil
}

// Verify checks a single beacon's signature.
func (v *Verifier) Verify(b *common.Beacon) error {
	return v.threshold.VerifyRecovered(v.pub, v.scheme.Digest(b), b.GetSignature())
}

// bls12381Suite returns the kyber v4 pairing suite for a circl/gnark backend and
// reports whether the scheme places signatures on G1. It returns an error wrapping
// ErrUnsupportedBackend when the scheme cannot be reproduced by these backends.
func bls12381Suite(backend, schemeName string) (suite pairing.Suite, sigOnG1 bool, err error) {
	switch schemeName {
	case crypto.DefaultSchemeID, crypto.UnchainedSchemeID:
		// keys on G1, signatures on G2, standard DSTs
		return newBLSSuite(backend), false, nil
	case crypto.SigsOnG1ID:
		// keys on G2, signatures on G1, standard RFC9380 DSTs
		return newBLSSuite(backend), true, nil
	case crypto.ShortSigSchemeID:
		return nil, false, fmt.Errorf("%w: %q uses the non-standard G2 DST on G1, only the kilic backend supports custom DSTs",
			ErrUnsupportedBackend, schemeName)
	case crypto.BN254UnchainedOnG1SchemeID:
		return nil, false, fmt.Errorf("%w: %q is on the bn254 curve, not BLS12-381", ErrUnsupportedBackend, schemeName)
	default:
		return nil, false, fmt.Errorf("unknown scheme %q", schemeName)
	}
}

func newBLSSuite(backend string) pairing.Suite {
	if backend == "gnark" {
		return gnark.NewSuite()
	}
	return circl.NewSuite()
}

// Result holds the outcome of verifying a whole store with a single backend.
type Result struct {
	Backend string
	// Unsupported is true when the backend cannot reproduce the chain's scheme.
	Unsupported bool
	// UnsupportedReason explains why the backend was skipped, if Unsupported.
	UnsupportedReason string
	// Verified is the number of beacons that verified successfully.
	Verified int
	// Failed is the number of beacons that failed verification.
	Failed int
	// FirstFailedRound is the round of the first failing beacon (only meaningful when Failed > 0).
	FirstFailedRound uint64
}

// OK reports whether the backend verified at least one beacon and had no failures.
func (r *Result) OK() bool {
	return !r.Unsupported && r.Failed == 0 && r.Verified > 0
}

// VerifyStore iterates over every beacon in store and verifies its signature
// against info.PublicKey using each requested backend. It returns one Result per
// backend, keyed by backend name. A backend that cannot reproduce the chain's
// scheme is reported as Unsupported rather than failing the whole run.
func VerifyStore(ctx context.Context, store chain.Store, info *chaininfo.Info, backends []string) (map[string]*Result, error) {
	rawPub, err := info.PublicKey.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshalling chain public key: %w", err)
	}

	results := make(map[string]*Result, len(backends))
	verifiers := make([]*Verifier, 0, len(backends))
	for _, backend := range backends {
		res := &Result{Backend: backend}
		results[backend] = res

		v, err := NewVerifier(info.Scheme, backend, rawPub)
		if err != nil {
			if errors.Is(err, ErrUnsupportedBackend) {
				res.Unsupported = true
				res.UnsupportedReason = err.Error()
				continue
			}
			return nil, fmt.Errorf("building %s verifier: %w", backend, err)
		}
		verifiers = append(verifiers, v)
	}

	err = store.Cursor(ctx, func(ctx context.Context, c chain.Cursor) error {
		b, err := c.First(ctx)
		for ; b != nil; b, err = c.Next(ctx) {
			if err != nil {
				return err
			}
			// Round 0 is the genesis seed: it carries no signature to verify.
			if b.Round == 0 {
				continue
			}
			for _, v := range verifiers {
				res := results[v.backend]
				if verr := v.Verify(b); verr != nil {
					if res.Failed == 0 {
						res.FirstFailedRound = b.Round
					}
					res.Failed++
				} else {
					res.Verified++
				}
			}
		}
		// Reaching the end of the database is signalled with ErrNoBeaconStored.
		if err != nil && !errors.Is(err, chainerrors.ErrNoBeaconStored) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return results, nil
}

// VerifyBoltDB opens the boltdb store located in folder (which must contain a
// drand.db file) and verifies all of its beacons against info using each backend.
// It transparently supports both the trimmed and untrimmed boltdb formats through
// the store abstraction.
func VerifyBoltDB(ctx context.Context, l log.Logger, folder string, info *chaininfo.Info, backends []string) (map[string]*Result, error) {
	store, err := boltdb.NewBoltStore(ctx, l, folder)
	if err != nil {
		return nil, fmt.Errorf("opening boltdb store in %q: %w", folder, err)
	}
	defer func() { _ = store.Close() }()

	return VerifyStore(ctx, store, info, backends)
}
