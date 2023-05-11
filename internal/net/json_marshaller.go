package net

import (
	"encoding/json"
	"io"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
)

// HexJSON transforms json into hex string instead of b64
type HexJSON struct{}

// ContentType always Returns "application/json".
func (*HexJSON) ContentType() string {
	return "application/json"
}

// Marshal marshals "v" into JSON
func (j *HexJSON) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal unmarshals JSON data into "v".
func (j *HexJSON) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// NewDecoder returns a Decoder which reads JSON stream from "r".
func (j *HexJSON) NewDecoder(r io.Reader) runtime.Decoder {
	return json.NewDecoder(r)
}

// NewEncoder returns an Encoder which writes JSON stream into "w".
func (j *HexJSON) NewEncoder(w io.Writer) runtime.Encoder {
	return json.NewEncoder(w)
}

// Delimiter for newline encoded JSON streams.
func (j *HexJSON) Delimiter() []byte {
	return []byte("\n")
}
