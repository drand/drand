package dkg

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/metrics"
	"github.com/drand/drand/internal/net"
	"github.com/drand/drand/internal/util"
	"github.com/drand/drand/protobuf/drand"
)

type GrpcNetwork struct {
	dkgClient net.DKGClient
	log       log.Logger
}

type SendAction func(ctx context.Context, client net.DKGClient, peer net.Peer) (*drand.EmptyResponse, error)

// Send currently sends sequentially (boo!)
// refactor this to fork join (and attempt all participants, in order that it can be used for rollbacks too)
func (n *GrpcNetwork) Send(
	ctx context.Context,
	from *drand.Participant,
	to []*drand.Participant,
	action SendAction,
) error {
	ctx, span := metrics.NewSpan(ctx, "grpc.Send")
	defer span.End()

	return n.send(ctx, from, to, action, false)
}

func (n *GrpcNetwork) SendIgnoringConnectionError(
	ctx context.Context,
	from *drand.Participant,
	to []*drand.Participant,
	action SendAction,
) error {
	ctx, span := metrics.NewSpan(ctx, "grpc.SendIgnoringConnectionError")
	defer span.End()

	return n.send(ctx, from, to, action, true)
}

// send forks a goroutine for each recipient and sends a network request to them
// in the case of error, it returns the first error
// if they are all successful, it returns nil
func (n *GrpcNetwork) send(
	ctx context.Context,
	from *drand.Participant,
	to []*drand.Participant,
	action SendAction,
	ignoreConnectionErrors bool,
) error {
	ctx, span := metrics.NewSpan(ctx, "grpc.send")
	defer span.End()

	wait := sync.WaitGroup{}
	errs := make(chan error, len(to))
	wait.Add(len(to))

	for _, p := range to {
		p := p

		if p.Address == from.Address {
			wait.Done()
			continue
		}

		go func() {
			defer wait.Done()
			_, err := action(ctx, n.dkgClient, util.ToPeer(p))
			if err != nil {
				if ignoreConnectionErrors && isConnectionError(err) {
					n.log.Warnw(fmt.Sprintf("connection error to node %s", p.Address), "err", err)
					return
				}
				errs <- err
			}
		}()
	}

	wait.Wait()

	select {
	case err := <-errs:
		return err
	default:
		return nil
	}
}

func isConnectionError(err error) bool {
	return strings.Contains(err.Error(), "Error while dialing dial")
}
