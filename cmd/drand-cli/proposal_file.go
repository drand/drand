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
	Address string
	TLS     bool
	Key     string
}

func (t *TomlParticipant) Into() *drand.Participant {
	return &drand.Participant{
		Address: t.Address,
		Tls:     t.TLS,
		PubKey:  decodeHexOrPanic(t.Key),
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
