package key

import (
	"encoding/hex"

	kyber "github.com/drand/kyber"
)

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
