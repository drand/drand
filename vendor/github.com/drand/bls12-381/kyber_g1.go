package bls

import (
	"crypto/cipher"
	"encoding/hex"
	"io"

	"github.com/drand/kyber"
	"github.com/drand/kyber/group/mod"
)

var domainG1 = [8]byte{1, 1, 1, 1, 1, 1, 1, 1}

// KyberG1 is a kyber.Point holding a G1 point on BLS12-381 curve
type KyberG1 struct {
	p *PointG1
}

func nullKyberG1() *KyberG1 {
	var p PointG1
	return newKyberG1(&p)
}
func newKyberG1(p *PointG1) *KyberG1 {
	return &KyberG1{p: p}
}

func (k *KyberG1) Equal(k2 kyber.Point) bool {
	return NewG1(nil).Equal(k.p, k2.(*KyberG1).p)
}

func (k *KyberG1) Null() kyber.Point {
	return newKyberG1(NewG1(nil).Zero())
}

func (k *KyberG1) Base() kyber.Point {
	return newKyberG1(NewG1(nil).One())
}

func (k *KyberG1) Pick(rand cipher.Stream) kyber.Point {
	//panic("not implemented")
	var dst, src [32]byte
	rand.XORKeyStream(dst[:], src[:])
	return k.Hash(dst[:])
}

func (k *KyberG1) Set(q kyber.Point) kyber.Point {
	k.p.Set(q.(*KyberG1).p)
	return k
}

func (k *KyberG1) Clone() kyber.Point {
	var p PointG1
	p.Set(k.p)
	return newKyberG1(&p)
}

func (k *KyberG1) EmbedLen() int {
	panic("bls12-381: unsupported operation")
}

func (k *KyberG1) Embed(data []byte, rand cipher.Stream) kyber.Point {
	panic("bls12-381: unsupported operation")
}

func (k *KyberG1) Data() ([]byte, error) {
	panic("bls12-381: unsupported operation")
}

func (k *KyberG1) Add(a, b kyber.Point) kyber.Point {
	aa := a.(*KyberG1)
	bb := b.(*KyberG1)
	NewG1(nil).Add(k.p, aa.p, bb.p)
	return k
}

func (k *KyberG1) Sub(a, b kyber.Point) kyber.Point {
	aa := a.(*KyberG1)
	bb := b.(*KyberG1)
	NewG1(nil).Sub(k.p, aa.p, bb.p)
	return k
}

func (k *KyberG1) Neg(a kyber.Point) kyber.Point {
	aa := a.(*KyberG1)
	NewG1(nil).Neg(k.p, aa.p)
	return k
}

func (k *KyberG1) Mul(s kyber.Scalar, q kyber.Point) kyber.Point {
	if q == nil {
		q = nullKyberG1().Base()
	}
	NewG1(nil).MulScalar(k.p, q.(*KyberG1).p, &s.(*mod.Int).V)
	return k
}

func (k *KyberG1) MarshalBinary() ([]byte, error) {
	return NewG1(nil).ToCompressed(k.p), nil
}

func (k *KyberG1) UnmarshalBinary(buff []byte) error {
	var err error
	k.p, err = NewG1(nil).FromCompressed(buff)
	return err
}

func (k *KyberG1) MarshalTo(w io.Writer) (int, error) {
	buf, err := k.MarshalBinary()
	if err != nil {
		return 0, err
	}
	return w.Write(buf)
}

func (k *KyberG1) UnmarshalFrom(r io.Reader) (int, error) {
	buf := make([]byte, k.MarshalSize())
	n, err := io.ReadFull(r, buf)
	if err != nil {
		return n, err
	}
	return n, k.UnmarshalBinary(buf)
}

func (k *KyberG1) MarshalSize() int {
	return 48
}

func (k *KyberG1) String() string {
	b, _ := k.MarshalBinary()
	return "bls12-381.G1: " + hex.EncodeToString(b)
}

func (k *KyberG1) Hash(m []byte) kyber.Point {
	if len(m) != 32 {
		m = sha256Hash(m)
	}
	var s [32]byte
	copy(s[:], m)
	pg1 := hashWithDomainG1(NewG1(nil), s, domainG2)
	k.p = pg1
	return k

}
