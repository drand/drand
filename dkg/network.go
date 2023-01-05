package dkg

import (
	"fmt"
	"strings"

	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/util"

	"github.com/drand/drand/protobuf/drand"
)

type GrpcNetwork struct {
	dkgClient net.DKGClient
	log       log.Logger
}

// Send currently sends sequentially (boo!)
// refactor this to fork join (and attempt all participants, in order that it can be used for rollbacks too)
func (n *GrpcNetwork) Send(
	from *drand.Participant,
	to []*drand.Participant,
	action func(client net.DKGClient, peer net.Peer) (*drand.EmptyResponse, error),
) error {
	return n.send(from, to, action, false)
}

func (n *GrpcNetwork) SendIgnoringConnectionError(
	from *drand.Participant,
	to []*drand.Participant,
	action func(client net.DKGClient, peer net.Peer) (*drand.EmptyResponse, error),
) error {
	return n.send(from, to, action, true)
}

func (n *GrpcNetwork) send(
	from *drand.Participant,
	to []*drand.Participant,
	action func(client net.DKGClient, peer net.Peer) (*drand.EmptyResponse, error),
	ignoreConnectionErrors bool,
) error {
	for _, p := range to {
		if p.Address == from.Address {
			continue
		}

		_, err := action(n.dkgClient, util.ToPeer(p))
		if err != nil {
			if ignoreConnectionErrors && isConnectionError(err) {
				n.log.Warnw(fmt.Sprintf("connection error to node %s", p.Address), "err", err)
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
