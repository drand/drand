package encoding

import (
	"bytes"
	"testing"

	"github.com/dedis/kyber/group/edwards25519"
	"github.com/dedis/kyber/util/random"
	"github.com/stretchr/testify/require"
)

var s = edwards25519.NewAES128SHA256Ed25519()

func ErrFatal(err error) {
	if err == nil {
		return
	}
	panic(err)
}

func TestPub64(t *testing.T) {
	b := &bytes.Buffer{}
	p := s.Point().Pick(random.Stream)
	ErrFatal(Write64Point(s, b, p))
	ErrFatal(Write64Point(s, b, p))
	p2, err := Read64Point(s, b)
	ErrFatal(err)
	require.EqualValues(t, p.String(), p2.String())

	p2, err = Read64Point(s, b)
	ErrFatal(err)
	require.Equal(t, p.String(), p2.String())
}

func TestScalar64(t *testing.T) {
	b := &bytes.Buffer{}
	sc := s.Scalar().Pick(random.Stream)
	ErrFatal(Write64Scalar(s, b, sc))
	ErrFatal(Write64Scalar(s, b, sc))
	s2, err := Read64Scalar(s, b)
	ErrFatal(err)
	require.True(t, sc.Equal(s2))
	s2, err = Read64Scalar(s, b)
	ErrFatal(err)
	require.True(t, sc.Equal(s2))
}

func TestPubHexStream(t *testing.T) {
	b := &bytes.Buffer{}
	p := s.Point().Pick(random.Stream)
	ErrFatal(WriteHexPoint(s, b, p))
	ErrFatal(WriteHexPoint(s, b, p))
	p2, err := ReadHexPoint(s, b)
	ErrFatal(err)
	require.Equal(t, p.String(), p2.String())
	p2, err = ReadHexPoint(s, b)
	ErrFatal(err)
	require.Equal(t, p.String(), p2.String())
}

func TestScalarHexStream(t *testing.T) {
	b := &bytes.Buffer{}
	sc := s.Scalar().Pick(random.Stream)
	ErrFatal(WriteHexScalar(s, b, sc))
	ErrFatal(WriteHexScalar(s, b, sc))
	s2, err := ReadHexScalar(s, b)
	ErrFatal(err)
	require.True(t, sc.Equal(s2))
	s2, err = ReadHexScalar(s, b)
	ErrFatal(err)
	require.True(t, sc.Equal(s2))
}

func TestPubHexString(t *testing.T) {
	p := s.Point().Pick(random.Stream)
	pstr, err := PointToStringHex(s, p)
	ErrFatal(err)
	p2, err := StringHexToPoint(s, pstr)
	ErrFatal(err)
	require.Equal(t, p.String(), p2.String())
}

func TestPub64String(t *testing.T) {
	p := s.Point().Pick(random.Stream)
	pstr, err := PointToString64(s, p)
	ErrFatal(err)
	p2, err := String64ToPoint(s, pstr)
	ErrFatal(err)
	require.Equal(t, p.String(), p2.String())
}

func TestScalarHexString(t *testing.T) {
	sc := s.Scalar().Pick(random.Stream)
	scstr, err := ScalarToStringHex(s, sc)
	ErrFatal(err)
	s2, err := StringHexToScalar(s, scstr)
	ErrFatal(err)
	require.True(t, sc.Equal(s2))
}

func TestScalar64String(t *testing.T) {
	sc := s.Scalar().Pick(random.Stream)
	scstr, err := ScalarToString64(s, sc)
	ErrFatal(err)
	s2, err := String64ToScalar(s, scstr)
	ErrFatal(err)
	require.True(t, sc.Equal(s2))
}
