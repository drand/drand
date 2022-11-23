package dkg

import (
	"errors"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
)

type GrpcNetwork struct {
}

// Send currently sends sequentially (boo!)
// refactor this to fork join (and attempt all participants, in order that it can be used for rollbacks too)
func (n *GrpcNetwork) Send(from *drand.Participant, to []*drand.Participant, action func(client drand.DKGClient) (*drand.GenericResponseMessage, error)) error {
	for _, p := range to {
		if p.Address == from.Address {
			continue
		}

		client, err := net.NewDKGClient(p.Address)
		if err != nil {
			return err
		}
		response, err := action(client)
		if err != nil {
			return err
		}
		if response.IsError {
			return errors.New(response.ErrorMessage)
		}
	}

	return nil
}
