package dkg

import (
	"strings"

	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
)

type GrpcNetwork struct {
}

// Send currently sends sequentially (boo!)
// refactor this to fork join (and attempt all participants, in order that it can be used for rollbacks too)
func (n *GrpcNetwork) Send(
	from *drand.Participant,
	to []*drand.Participant,
	action func(client drand.DKGClient) (*drand.EmptyResponse, error),
) error {
	return n.send(from, to, action, false)
}

func (n *GrpcNetwork) SendIgnoringConnectionError(
	from *drand.Participant,
	to []*drand.Participant,
	action func(client drand.DKGClient) (*drand.EmptyResponse, error),
) error {
	return n.send(from, to, action, true)
}

func (n *GrpcNetwork) send(
	from *drand.Participant,
	to []*drand.Participant,
	action func(client drand.DKGClient) (*drand.EmptyResponse, error),
	ignoreConnectionErrors bool,
) error {
	for _, p := range to {
		if p.Address == from.Address {
			continue
		}

		client, err := net.NewDKGClient(p.Address, p.Tls)
		if err != nil {
			return err
		}
		_, err = action(client)
		if err != nil {
			if ignoreConnectionErrors && isConnectionError(err) {
				continue
			}
			return err
		}
	}

	return nil
}

func isConnectionError(err error) bool {
	return strings.Contains(err.Error(), "Error while dialing dial")
}
