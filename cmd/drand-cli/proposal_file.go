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
	out := make([]*drand.Participant, len(p.Joining))

	for i, participant := range p.Joining {
		out[i] = participant.Into()
	}

	return out
}
func (p *ProposalFile) Leavers() []*drand.Participant {
	out := make([]*drand.Participant, len(p.Leaving))

	for i, participant := range p.Leaving {
		out[i] = participant.Into()
	}

	return out
}

func (p *ProposalFile) Remainers() []*drand.Participant {
	out := make([]*drand.Participant, len(p.Remaining))

	for i, participant := range p.Remaining {
		out[i] = participant.Into()
	}

	return out
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
