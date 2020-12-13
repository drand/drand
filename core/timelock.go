package core

import (
	"errors"

	"golang.org/x/crypto/blake2s"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/kyber"
	"github.com/drand/kyber/util/random"
)

type Signature struct {
	rp  kyber.Point
	xor []byte
}

// TODO make that public in kyber
type hashablePoint interface {
	Hash([]byte) kyber.Point
}

// SigGroup = G2
// KeyGroup = G1
// P random generator of G1
// dist key: s, Ppub = s*P \in G1
// H1: {0,1}^n -> G1
// H2: GT -> {0,1}^n
// ID is round
// ID: Qid = H1(ID) = xP \in G2
// 	secret did = s*Qid \in G2
// Encrypt:
// - random r scalar
// - Gid = e(Ppub, r*Qid) = e(P, P)^(x*s*r) \in GT
// 		 = GidT
// - U = rP \in G1,
// - V = M XOR H2(Gid)) = M XOR H2(GidT)  \in {0,1}^n
// Decrypt:
// - V XOR H2(e(U, did)) = V XOR H2(e(rP, s*Qid))
//   = V XOR H2(e(P, P)^(r*s*x))
//   = V XOR H2(GidT) = M
func Encrypt(public *key.DistPublic, round uint64, msg []byte) (*Signature, error) {
	toMsg := chain.Message(round)
	hashable, ok := key.SigGroup.Point().(hashablePoint)
	if !ok {
		return nil, errors.New("point needs to implement hashablePoint")
	}
	Qid := hashable.Hash(toMsg)
	r := key.SigGroup.Scalar().Pick(random.New())
	rP := key.KeyGroup.Point().Mul(r, public.BasePoint)

	// e(Qid, Ppub) = e( H(round), s*P) where s is dist secret key
	Ppub := public.Key()
	rQid := key.SigGroup.Point().Mul(r, Qid)
	GidT := key.Pairing.Pair(Ppub, rQid)
	// TODO check on size
	// H(gid)
	hGidT := gtToHash(GidT, uint16(len(msg)))
	xored := xor(msg, hGidT)

	return &Signature{
		rp:  rP,
		xor: xored,
	}, nil
}

// private is in G2
func Decrypt(private kyber.Point, s *Signature) []byte {
	gidt := key.Pairing.Pair(s.rp, private)
	hgidt := gtToHash(gidt, uint16(len(s.xor)))
	return xor(s.xor, hgidt)
}

func gtToHash(gt kyber.Point, length uint16) []byte {
	xof, err := blake2s.NewXOF(length, nil)
	if err != nil {
		panic(err)
	}
	gt.MarshalTo(xof)
	var b = make([]byte, length)
	n, err := xof.Read(b)
	if uint16(n) != length || err != nil {
		panic("stg wrong with hash")
	}
	return b[:]
}

func xor(a, b []byte) []byte {
	if len(a) != len(b) {
		panic("wrong xor input")
	}
	res := make([]byte, len(a))
	for i := 0; i < len(a); i++ {
		res[i] = a[i] ^ b[i]
	}
	return res
}
