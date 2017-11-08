// Package encoding package provides many methods to encode/decode a Point/Scalar in
// base64 or hexadecimal.
//
// This package is useful when dealing with custom encoding that are not able to process
// any interfaces such as Point. It is first needed to encode a Point in its string equivalent.
package encoding

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strings"

	"github.com/dedis/kyber"
)

// Read64Point reads a point from a base64 representation
func Read64Point(group kyber.Group, r io.Reader) (kyber.Point, error) {
	point := group.Point()
	dec := base64.NewDecoder(base64.StdEncoding, r)
	_, err := point.UnmarshalFrom(dec)
	return point, err
}

// Write64Point writes a point to a base64 representation
func Write64Point(group kyber.Group, w io.Writer, point kyber.Point) error {
	enc := base64.NewEncoder(base64.StdEncoding, w)
	return write64(enc, point)
}

// Read64Scalar takes a Base64-encoded scalar and returns that scalar,
// optionally an error
func Read64Scalar(group kyber.Group, r io.Reader) (kyber.Scalar, error) {
	s := group.Scalar()
	dec := base64.NewDecoder(base64.StdEncoding, r)
	_, err := s.UnmarshalFrom(dec)
	return s, err
}

// Write64Scalar converts a scalar key to a Base64-string
func Write64Scalar(group kyber.Group, w io.Writer, scalar kyber.Scalar) error {
	enc := base64.NewEncoder(base64.StdEncoding, w)
	return write64(enc, scalar)
}

// ReadHexPoint reads a point from a hex representation
func ReadHexPoint(group kyber.Group, r io.Reader) (kyber.Point, error) {
	point := group.Point()
	buf, err := getHex(r, point.MarshalSize())
	if err != nil {
		return nil, err
	}
	err = point.UnmarshalBinary(buf)
	return point, err
}

// WriteHexPoint writes a point to a hex representation
func WriteHexPoint(group kyber.Group, w io.Writer, point kyber.Point) error {
	buf, err := point.MarshalBinary()
	if err != nil {
		return err
	}
	out := hex.EncodeToString(buf)
	_, err = w.Write([]byte(out))
	return err
}

// ReadHexScalar takes a hex-encoded scalar and returns that scalar,
// optionally an error
func ReadHexScalar(group kyber.Group, r io.Reader) (kyber.Scalar, error) {
	s := group.Scalar()
	buf, err := getHex(r, s.MarshalSize())
	if err != nil {
		return nil, err
	}
	s.UnmarshalBinary(buf)
	return s, nil
}

// WriteHexScalar converts a scalar key to a hex-string
func WriteHexScalar(group kyber.Group, w io.Writer, scalar kyber.Scalar) error {
	buf, err := scalar.MarshalBinary()
	if err != nil {
		return err
	}
	out := hex.EncodeToString(buf)
	_, err = w.Write([]byte(out))
	return err
}

// PointToStringHex converts a point to a hexadecimal representation
func PointToStringHex(group kyber.Group, point kyber.Point) (string, error) {
	pbuf, err := point.MarshalBinary()
	return hex.EncodeToString(pbuf), err
}

// StringHexToPoint reads a hexadecimal representation of a point and convert it to the
// right struct
func StringHexToPoint(group kyber.Group, s string) (kyber.Point, error) {
	return ReadHexPoint(group, strings.NewReader(s))
}

// PointToString64 converts a point to a base64 representation
func PointToString64(group kyber.Group, point kyber.Point) (string, error) {
	pbuf, err := point.MarshalBinary()
	return base64.StdEncoding.EncodeToString(pbuf), err
}

// String64ToPoint reads a base64 representation of a point and converts it
// back to a point.
func String64ToPoint(group kyber.Group, s string) (kyber.Point, error) {
	return Read64Point(group, strings.NewReader(s))
}

// ScalarToStringHex encodes a scalar to hexadecimal
func ScalarToStringHex(group kyber.Group, scalar kyber.Scalar) (string, error) {
	sbuf, err := scalar.MarshalBinary()
	return hex.EncodeToString(sbuf), err
}

// StringHexToScalar reads a scalar in hexadecimal from string
func StringHexToScalar(group kyber.Group, str string) (kyber.Scalar, error) {
	return ReadHexScalar(group, strings.NewReader(str))
}

// ScalarToString64 encodes a scalar to a base64
func ScalarToString64(group kyber.Group, scalar kyber.Scalar) (string, error) {
	sbuf, err := scalar.MarshalBinary()
	return base64.StdEncoding.EncodeToString(sbuf), err
}

// String64ToScalar reads a scalar in base64 from a string
func String64ToScalar(group kyber.Group, str string) (kyber.Scalar, error) {
	return Read64Scalar(group, strings.NewReader(str))
}

func write64(wc io.WriteCloser, data ...kyber.Marshaling) error {
	for _, d := range data {
		if _, err := d.MarshalTo(wc); err != nil {
			return err
		}
	}
	return wc.Close()
}

func getHex(r io.Reader, len int) ([]byte, error) {
	bufHex := make([]byte, len*2)
	bufByte := make([]byte, len)
	l, err := r.Read(bufHex)
	if err != nil {
		return nil, err
	}
	if l < len {
		return nil, errors.New("didn't get enough bytes from stream")
	}
	_, err = hex.Decode(bufByte, bufHex)
	if err != nil {
		return nil, err
	}
	return bufByte, nil
}
