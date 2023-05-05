package test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/drand/drand/common/key"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/dkg"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"github.com/drand/drand/protobuf/drand"
)

type DKGRunner struct {
	Client   drand.DKGControlClient
	BeaconID string
}

func (r *DKGRunner) StartNetwork(
	threshold int,
	period int,
	schemeID string,
	timeout time.Duration,
	catchupPeriod int,
	joiners []*drand.Participant,
) error {
	_, err := r.Client.Command(context.Background(), &drand.DKGCommand{Command: &drand.DKGCommand_Initial{
		Initial: &drand.FirstProposalOptions{
			BeaconID:             r.BeaconID,
			Timeout:              timestamppb.New(time.Now().Add(timeout)),
			Threshold:            uint32(threshold),
			PeriodSeconds:        uint32(period),
			Scheme:               schemeID,
			CatchupPeriodSeconds: uint32(catchupPeriod),
			// put the genesis a little in the future to give demo nodes some time to do the DKG
			GenesisTime: timestamppb.New(time.Now().Add(10 * time.Second)),
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
	_, err := r.Client.Command(context.Background(), &drand.DKGCommand{Command: &drand.DKGCommand_Resharing{
		Resharing: &drand.ProposalOptions{
			BeaconID:             r.BeaconID,
			Threshold:            uint32(threshold),
			CatchupPeriodSeconds: uint32(catchupPeriod),
			Timeout:              timestamppb.New(time.Now().Add(1 * time.Minute)),
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
	_, err := r.Client.Command(context.Background(), &drand.DKGCommand{Command: &drand.DKGCommand_Execute{
		Execute: &drand.ExecutionOptions{BeaconID: r.BeaconID}},
		Metadata: &drand.CommandMetadata{BeaconID: r.BeaconID},
	})
	return err
}

func (r *DKGRunner) JoinDKG() error {
	_, err := r.Client.Command(context.Background(), &drand.DKGCommand{Command: &drand.DKGCommand_Join{
		Join: &drand.JoinOptions{
			BeaconID:  r.BeaconID,
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
	_, err = r.Client.Command(context.Background(), &drand.DKGCommand{Command: &drand.DKGCommand_Join{
		Join: &drand.JoinOptions{
			BeaconID:  r.BeaconID,
			GroupFile: groupFileBytes.Bytes(),
		}},
		Metadata: &drand.CommandMetadata{BeaconID: r.BeaconID},
	})

	return err
}

func (r *DKGRunner) Accept() error {
	_, err := r.Client.Command(context.Background(), &drand.DKGCommand{Command: &drand.DKGCommand_Accept{
		Accept: &drand.AcceptOptions{
			BeaconID: r.BeaconID,
		}},
		Metadata: &drand.CommandMetadata{BeaconID: r.BeaconID},
	})
	return err
}

func (r *DKGRunner) Abort() error {
	_, err := r.Client.Command(context.Background(), &drand.DKGCommand{Command: &drand.DKGCommand_Abort{
		Abort: &drand.AbortOptions{BeaconID: r.BeaconID}},
		Metadata: &drand.CommandMetadata{BeaconID: r.BeaconID},
	})

	return err
}

var ErrTimeout = errors.New("DKG timed out")

func (r *DKGRunner) WaitForDKG(lg log.Logger, beaconID string, epoch uint32, numberOfSeconds int) error {
	waitForGroupFile := func() error {
		res, err := r.Client.DKGStatus(context.Background(), &drand.DKGStatusRequest{BeaconID: beaconID})
		if err != nil {
			return err
		}

		switch res.Current.State {
		case uint32(dkg.Evicted):
			panic("leader got evicted")
		case uint32(dkg.TimedOut):
			panic("DKG timed out")
		case uint32(dkg.Aborted):
			panic("DKG was aborted")
		}

		if res.Complete == nil || res.Complete.Epoch != epoch {
			return errors.New("DKG not finished yet")
		}

		if res.Complete.State == uint32(dkg.TimedOut) {
			return ErrTimeout
		}

		if res.Complete.State != uint32(dkg.Complete) {
			panic(fmt.Sprintf("leader completed DKG in unexpected state: %s", dkg.Status(res.Complete.State).String()))
		}
		return nil
	}

	var err error
	for i := 0; i < numberOfSeconds; i++ {
		err = waitForGroupFile()
		if err == nil {
			break
		}
		lg.Infow("DKG not finished... retrying")
		time.Sleep(1 * time.Second)
	}

	return err
}
