package node

import (
	"bytes"
	"encoding/hex"
	"os"

	"github.com/BurntSushi/toml"
	cli "github.com/drand/drand/cmd/drand-cli"
	"github.com/drand/drand/protobuf/drand"
)

type ProposalFile struct {
	Joining   []*drand.Participant
	Leaving   []*drand.Participant
	Remaining []*drand.Participant
}

func WriteProposalFile(filepath string, proposal ProposalFile) error {
	file := cli.ProposalFileFormat{
		Joining:   mapEach(proposal.Joining, toTomlModel),
		Remaining: mapEach(proposal.Remaining, toTomlModel),
		Leaving:   mapEach(proposal.Leaving, toTomlModel),
	}

	var buff bytes.Buffer
	err := toml.NewEncoder(&buff).Encode(file)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath, buff.Bytes(), 0755)
}

func mapEach(ps []*drand.Participant, fn func(participant *drand.Participant) *cli.TomlParticipant) []*cli.TomlParticipant {
	out := make([]*cli.TomlParticipant, len(ps))
	for i, value := range ps {
		out[i] = fn(value)
	}
	return out
}

func toTomlModel(p *drand.Participant) *cli.TomlParticipant {
	return &cli.TomlParticipant{
		Address:   p.Address,
		TLS:       p.Tls,
		Key:       hex.EncodeToString(p.PubKey),
		Signature: hex.EncodeToString(p.Signature),
	}
}
