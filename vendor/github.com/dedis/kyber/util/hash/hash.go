// Package hash provides utility functions to process complex data types, like
// data streams, files, or sequences of structs of different types.
package hash

import (
	"bytes"
	"errors"
	"hash"
	"io"
	"os"

	"encoding"
	"reflect"
)

// Bytes returns the hash of all given byte slices.
func Bytes(hash hash.Hash, bytes ...[]byte) ([]byte, error) {
	for _, slice := range bytes {
		if _, err := hash.Write(slice); err != nil {
			return nil, err
		}
	}
	return hash.Sum(nil), nil
}

// Stream returns the hash of a data stream.
func Stream(hash hash.Hash, stream io.Reader) ([]byte, error) {
	if _, err := io.Copy(hash, stream); err != nil {
		return nil, err
	}
	return hash.Sum(nil), nil
}

// File returns the hash of a file.
func File(hash hash.Hash, file string) ([]byte, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Stream(hash, f)
}

// Structures returns the hash of all the given arguments. Each argument has to
// implement the BinaryMarshaler interface.
func Structures(hash hash.Hash, args ...interface{}) ([]byte, error) {
	var res, buf []byte
	bmArgs, err := convertToBinaryMarshaler(args)
	if err != nil {
		return nil, err
	}
	for _, a := range bmArgs {
		buf, err = a.MarshalBinary()
		if err != nil {
			return nil, err
		}
		res, err = Stream(hash, bytes.NewReader(buf))
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

// convertToBinaryMarshaler takes a slice of interfaces and returns
// a slice of BinaryMarshalers.
func convertToBinaryMarshaler(args ...interface{}) ([]encoding.BinaryMarshaler, error) {
	var ret []encoding.BinaryMarshaler
	for _, a := range args {
		refl := reflect.ValueOf(a)
		if refl.Kind() == reflect.Slice {
			for b := 0; b < refl.Len(); b++ {
				el := refl.Index(b)
				bms, err := convertToBinaryMarshaler(el.Interface())
				if err != nil {
					return nil, err
				}
				ret = append(ret, bms...)
			}
		} else {
			bm, ok := a.(encoding.BinaryMarshaler)
			if !ok {
				return nil, errors.New("Could not convert to BinaryMarshaler")
			}
			ret = append(ret, bm)
		}
	}
	return ret, nil
}
