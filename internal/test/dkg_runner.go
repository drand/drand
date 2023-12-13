package test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	clock "github.com/jonboulle/clockwork"

	"github.com/BurntSushi/toml"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/drand/drand/common/key"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/dkg"
	drand "github.com/drand/drand/protobuf/dkg"
)

type DKGRunner struct {
	Client   drand.DKGControlClient
	BeaconID string
	Clock    clock.Clock
}

func (r *DKGRunner) StartNetwork(
	threshold int,
	period int,
	schemeID string,
	timeout time.Duration,
	catchupPeriod int,
	joiners []*drand.Participant,
) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := r.Client.Command(ctx, &drand.DKGCommand{Command: &drand.DKGCommand_Initial{
		Initial: &drand.FirstProposalOptions{
			Timeout:              timestamppb.New(r.Clock.Now().Add(timeout)),
			Threshold:            uint32(threshold),
			PeriodSeconds:        uint32(period),
			Scheme:               schemeID,
			CatchupPeriodSeconds: uint32(catchupPeriod),
			// put the genesis a little in the future to give demo nodes some time to do the DKG
			GenesisTime: timestamppb.New(r.Clock.Now().Add(20 * time.Second)),
			Joining:     joiners,
		},
	},
		Metadata: &drand.CommandMetadata{BeaconID: r.BeaconID},
	})
	return err
}

func (r *DKGRunner) StartProposal(
	threshold int,
	transitionTime time.Time,
	catchupPeriod int,
	joiners []*drand.Participant,
	remainers []*drand.Participant,
	leavers []*drand.Participant,
) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := r.Client.Command(ctx, &drand.DKGCommand{Command: &drand.DKGCommand_Resharing{
		Resharing: &drand.ProposalOptions{
			Threshold:            uint32(threshold),
			CatchupPeriodSeconds: uint32(catchupPeriod),
			Timeout:              timestamppb.New(r.Clock.Now().Add(1 * time.Minute)),
			TransitionTime:       timestamppb.New(transitionTime),
			Joining:              joiners,
			Remaining:            remainers,
			Leaving:              leavers,
		}},
		Metadata: &drand.CommandMetadata{BeaconID: r.BeaconID},
	})
	return err
}

func (r *DKGRunner) StartExecution() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := r.Client.Command(ctx, &drand.DKGCommand{Command: &drand.DKGCommand_Execute{
		Execute: &drand.ExecutionOptions{}},
		Metadata: &drand.CommandMetadata{BeaconID: r.BeaconID},
	})
	return err
}

func (r *DKGRunner) JoinDKG() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := r.Client.Command(ctx, &drand.DKGCommand{Command: &drand.DKGCommand_Join{
		Join: &drand.JoinOptions{
			GroupFile: nil,
		}},
		Metadata: &drand.CommandMetadata{BeaconID: r.BeaconID},
	})

	return err
}

func (r *DKGRunner) JoinReshare(oldGroup *key.Group) error {
	var groupFileBytes bytes.Buffer
	err := toml.NewEncoder(&groupFileBytes).Encode(oldGroup.TOML())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err = r.Client.Command(ctx, &drand.DKGCommand{Command: &drand.DKGCommand_Join{
		Join: &drand.JoinOptions{
			GroupFile: groupFileBytes.Bytes(),
		}},
		Metadata: &drand.CommandMetadata{BeaconID: r.BeaconID},
	})

	return err
}

func (r *DKGRunner) Accept() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := r.Client.Command(ctx, &drand.DKGCommand{Command: &drand.DKGCommand_Accept{
		Accept: &drand.AcceptOptions{}},
		Metadata: &drand.CommandMetadata{BeaconID: r.BeaconID},
	})
	return err
}

func (r *DKGRunner) Abort() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := r.Client.Command(ctx, &drand.DKGCommand{Command: &drand.DKGCommand_Abort{
		Abort: &drand.AbortOptions{}},
		Metadata: &drand.CommandMetadata{BeaconID: r.BeaconID},
	})

	return err
}

var ErrTimeout = errors.New("DKG timed out")
var ErrDKGFailed = errors.New("DKG failed")

func (r *DKGRunner) WaitForDKG(lg log.Logger, beaconID string, epoch uint32, numberOfSeconds int) error {
	for i := 0; i < numberOfSeconds; i++ {
		time.Sleep(1 * time.Second)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		res, err := r.Client.DKGStatus(ctx, &drand.DKGStatusRequest{BeaconID: beaconID})
		if err != nil {
			continue
		}

		switch res.Current.State {
		case uint32(dkg.Evicted):
			return errors.New("leader got evicted")
		case uint32(dkg.TimedOut):
			return ErrTimeout
		case uint32(dkg.Aborted):
			return errors.New("DKG aborted")
		case uint32(dkg.Failed):
			return ErrDKGFailed
		}
		if res.Complete == nil || res.Complete.Epoch != epoch {
			continue
		}

		if res.Complete.State != uint32(dkg.Complete) {
			panic(fmt.Sprintf("leader completed DKG in unexpected state: %s", dkg.Status(res.Complete.State).String()))
		}

		if err == nil {
			return nil
		}
		lg.Infow("DKG not finished... retrying")
	}

	return errors.New("DKG never finished")
}
