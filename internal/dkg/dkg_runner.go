package dkg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	drand "github.com/drand/drand/protobuf/dkg"
	clock "github.com/jonboulle/clockwork"
	"time"

	"github.com/BurntSushi/toml"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/drand/drand/common/key"
	"github.com/drand/drand/common/log"
)

const GenesisDelay = 20 * time.Second

// TestRunner is a convenience struct for running DKG tests
type TestRunner struct {
	Client   drand.DKGControlClient
	BeaconID string
	Clock    clock.Clock
}

func (r *TestRunner) StartNetwork(
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
			GenesisTime: timestamppb.New(r.Clock.Now().Add(GenesisDelay)),
			Joining:     joiners,
		},
	},
		Metadata: &drand.CommandMetadata{BeaconID: r.BeaconID},
	})
	return err
}

func (r *TestRunner) StartReshare(
	threshold int,
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
			Joining:              joiners,
			Remaining:            remainers,
			Leaving:              leavers,
		}},
		Metadata: &drand.CommandMetadata{BeaconID: r.BeaconID},
	})
	return err
}

func (r *TestRunner) StartExecution() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := r.Client.Command(ctx, &drand.DKGCommand{Command: &drand.DKGCommand_Execute{
		Execute: &drand.ExecutionOptions{}},
		Metadata: &drand.CommandMetadata{BeaconID: r.BeaconID},
	})
	return err
}

func (r *TestRunner) JoinDKG() error {
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

func (r *TestRunner) JoinReshare(oldGroup *key.Group) error {
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

func (r *TestRunner) Accept() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := r.Client.Command(ctx, &drand.DKGCommand{Command: &drand.DKGCommand_Accept{
		Accept: &drand.AcceptOptions{}},
		Metadata: &drand.CommandMetadata{BeaconID: r.BeaconID},
	})
	return err
}

func (r *TestRunner) Abort() error {
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
var ErrDKGAborted = errors.New("DKG aborted")

func (r *TestRunner) WaitForDKG(lg log.Logger, epoch uint32, numberOfSeconds int) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < numberOfSeconds; i++ {
		time.Sleep(1 * time.Second)
		res, err := r.Client.DKGStatus(ctx, &drand.DKGStatusRequest{BeaconID: r.BeaconID})
		if err != nil {
			continue
		}

		switch res.Current.State {
		case uint32(TimedOut):
			return ErrTimeout
		case uint32(Aborted):
			return ErrDKGAborted
		case uint32(Failed):
			return ErrDKGFailed
		}
		if res.Complete == nil || res.Complete.Epoch != epoch {
			continue
		}

		if res.Complete.State != uint32(Complete) {
			panic(fmt.Sprintf("leader completed DKG in unexpected state: %s", Status(res.Complete.State).String()))
		}

		if err == nil {
			return nil
		}
		lg.Infow("DKG not finished... retrying")
	}

	return errors.New("DKG never finished")
}
