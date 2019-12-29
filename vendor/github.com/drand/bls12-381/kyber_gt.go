package bls

import (
	"crypto/cipher"
	"encoding/hex"
	"io"

	"github.com/drand/kyber"
	"golang.org/x/crypto/blake2b"
)

type KyberGT struct {
	f *fe12
}

func newEmptyGT() *KyberGT {
	return newKyberGT(newFp12(nil).newElement())
}
func newKyberGT(f *fe12) *KyberGT {
	return &KyberGT{
		f: f,
	}
}

func (k *KyberGT) Equal(kk kyber.Point) bool {
	return newFp12(nil).equal(k.f, kk.(*KyberGT).f)
}

func (k *KyberGT) Null() kyber.Point {
	k.f = newFp12(nil).zero()
	return k
}

func (k *KyberGT) Base() kyber.Point {
	var baseReader, _ = blake2b.NewXOF(0, []byte("Quand il y a à manger pour huit, il y en a bien pour dix."))
	_, err := newFp12(nil).randElement(k.f, baseReader)
	if err != nil {
		panic(err)
	}
	return k
}

func (k *KyberGT) Pick(rand cipher.Stream) kyber.Point {
	panic("TODO: bls12-381.GT.Pick()")
}

func (k *KyberGT) Set(q kyber.Point) kyber.Point {
	newFp12(nil).copy(k.f, q.(*KyberGT).f)
	return k
}

func (k *KyberGT) Clone() kyber.Point {
	kk := newEmptyGT()
	kk.Set(k)
	return kk
}

func (k *KyberGT) Add(a, b kyber.Point) kyber.Point {
	/* aa := a.(*KyberGT)*/
	//bb := b.(*KyberGT)
	//newFp12(nil).mul(k.f, aa.f, bb.f)
	/*return k*/
	panic("bls12-381: GT is not a full kyber.Point implementation")
}

func (k *KyberGT) Sub(a, b kyber.Point) kyber.Point {
	//return k.Add(k.Neg(k), b)
	panic("bls12-381: GT is not a full kyber.Point implementation")
}

func (k *KyberGT) Neg(q kyber.Point) kyber.Point {
	/*x := q.(*KyberGT)*/
	//newFp12(nil).neg(k.f, x.f)
	/*return k*/
	panic("bls12-381: GT is not a full kyber.Point implementation")
}

func (k *KyberGT) Mul(s kyber.Scalar, q kyber.Point) kyber.Point {
	panic("bls12-381: GT is not a full kyber.Point implementation")
	/*if q == nil {*/
	//q = newEmptyGT().Base()
	//}
	//scalar := &s.(*mod.Int).V
	//newFp12(nil).exp(k.f, q.(*KyberGT).f, scalar)
	//fmt.Printf("Kyber.GT.f.mul(-bitlen %d- %v, %v)): %v\n", scalar.BitLen(), scalar, q.(*KyberGT).f, k.f)
	/*return k*/
}

func (k *KyberGT) MarshalBinary() ([]byte, error) {
	return newFp12(nil).toBytes(k.f), nil
}

func (k *KyberGT) MarshalTo(w io.Writer) (int, error) {
	buf, err := k.MarshalBinary()
	if err != nil {
		return 0, err
	}
	return w.Write(buf)
}

func (k *KyberGT) UnmarshalBinary(buf []byte) error {
	return newFp12(nil).newElementFromBytes(k.f, buf)
}

func (k *KyberGT) UnmarshalFrom(r io.Reader) (int, error) {
	buf := make([]byte, k.MarshalSize())
	n, err := io.ReadFull(r, buf)
	if err != nil {
		return n, err
	}
	return n, k.UnmarshalBinary(buf)
}

func (k *KyberGT) MarshalSize() int {
	return 576
}

func (k *KyberGT) String() string {
	b, _ := k.MarshalBinary()
	return "bls12-381.GT: " + hex.EncodeToString(b)
}

func (k *KyberGT) EmbedLen() int {
	panic("bls12-381.GT.EmbedLen(): unsupported operation")
}

func (k *KyberGT) Embed(data []byte, rand cipher.Stream) kyber.Point {
	panic("bls12-381.GT.Embed(): unsupported operation")
}

func (k *KyberGT) Data() ([]byte, error) {
	panic("bls12-381.GT.Data(): unsupported operation")
}
