package drand

import (
	"encoding/hex"
	"github.com/drand/drand/protobuf/drand"
)

type ProposalFile struct {
	Joining   []*TomlParticipant
	Leaving   []*TomlParticipant
	Remaining []*TomlParticipant
}

func (p *ProposalFile) Joiners() []*drand.Participant {
	return mapEach(p.Joining, func(p *TomlParticipant) *drand.Participant {
		return p.Into()
	})
}
func (p *ProposalFile) Leavers() []*drand.Participant {
	return mapEach(p.Leaving, func(p *TomlParticipant) *drand.Participant {
		return p.Into()
	})
}

func (p *ProposalFile) Remainers() []*drand.Participant {
	return mapEach(p.Remaining, func(p *TomlParticipant) *drand.Participant {
		return p.Into()
	})
}

type TomlParticipant struct {
	Address   string
	TLS       bool
	Key       string
	Signature string
}

func (t *TomlParticipant) Into() *drand.Participant {
	return &drand.Participant{
		Address:   t.Address,
		Tls:       t.TLS,
		PubKey:    decodeHexOrPanic(t.Key),
		Signature: decodeHexOrPanic(t.Signature),
	}
}

func decodeHexOrPanic(input string) []byte {
	// input lengths are u8 instead of hex, so /2
	out := make([]byte, len(input)/2)

	_, err := hex.Decode(out, []byte(input))
	if err != nil {
		panic("Invalid hex in proposal file!")
	}

	return out
}

func mapEach[T any, U any](arr []T, fn func(T) U) []U {
	if arr == nil {
		return nil
	}
	out := make([]U, len(arr))

	for _, v := range arr {
		out = append(out, fn(v))
	}

	return out
}
