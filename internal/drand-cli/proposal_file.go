package drand

import (
	"encoding/hex"

	"github.com/BurntSushi/toml"

	"github.com/drand/drand/protobuf/drand"
)

type ProposalFileFormat struct {
	Joining   []*TomlParticipant
	Leaving   []*TomlParticipant
	Remaining []*TomlParticipant
}

type ProposalFile struct {
	Joining   []*drand.Participant
	Leaving   []*drand.Participant
	Remaining []*drand.Participant
}

func ParseProposalFile(filepath string) (*ProposalFile, error) {
	proposalFile := ProposalFileFormat{}
	_, err := toml.DecodeFile(filepath, &proposalFile)

	if err != nil {
		return nil, err
	}

	return &ProposalFile{
		Joining:   proposalFile.Joiners(),
		Leaving:   proposalFile.Leavers(),
		Remaining: proposalFile.Remainers(),
	}, nil
}

func (p *ProposalFileFormat) Joiners() []*drand.Participant {
	out := make([]*drand.Participant, len(p.Joining))

	for i, participant := range p.Joining {
		out[i] = participant.Into()
	}

	return out
}
func (p *ProposalFileFormat) Leavers() []*drand.Participant {
	out := make([]*drand.Participant, len(p.Leaving))

	for i, participant := range p.Leaving {
		out[i] = participant.Into()
	}

	return out
}

func (p *ProposalFileFormat) Remainers() []*drand.Participant {
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
		Key:       decodeHexOrPanic(t.Key),
		Signature: decodeHexOrPanic(t.Signature),
	}
}

func (p *ProposalFile) TOML() ProposalFileFormat {
	out := ProposalFileFormat{}

	for _, j := range p.Joining {
		p := encode(j)
		out.Joining = append(out.Joining, &p)
	}

	for _, r := range p.Remaining {
		p := encode(r)
		out.Remaining = append(out.Remaining, &p)
	}

	for _, l := range p.Leaving {
		p := encode(l)
		out.Leaving = append(out.Leaving, &p)
	}

	return out
}

func encode(p *drand.Participant) TomlParticipant {
	return TomlParticipant{
		Address:   p.Address,
		TLS:       p.Tls,
		Key:       hex.EncodeToString(p.Key),
		Signature: hex.EncodeToString(p.Signature),
	}
}

func decodeHexOrPanic(input string) []byte {
	// input lengths are u8 instead of hex, so /2
	byteLength := len(input) / 2
	out := make([]byte, byteLength)

	_, err := hex.Decode(out, []byte(input))
	if err != nil {
		panic("Invalid hex in proposal file!")
	}

	return out
}
