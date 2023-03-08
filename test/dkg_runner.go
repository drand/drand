package test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/drand/drand/dkg"
	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
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
	_, err := r.Client.StartNetwork(context.Background(), &drand.FirstProposalOptions{
		BeaconID:             r.BeaconID,
		Timeout:              timestamppb.New(time.Now().Add(timeout)),
		Threshold:            uint32(threshold),
		PeriodSeconds:        uint32(period),
		Scheme:               schemeID,
		CatchupPeriodSeconds: uint32(catchupPeriod),
		// put the genesis a little in the future to give demo nodes some time to do the DKG
		GenesisTime: timestamppb.New(time.Now().Add(10 * time.Second)),
		Joining:     joiners,
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
	_, err := r.Client.StartProposal(context.Background(), &drand.ProposalOptions{
		BeaconID:             r.BeaconID,
		Threshold:            uint32(threshold),
		CatchupPeriodSeconds: uint32(catchupPeriod),
		Timeout:              timestamppb.New(time.Now().Add(1 * time.Minute)),
		TransitionTime:       timestamppb.New(transitionTime),
		Joining:              joiners,
		Remaining:            remainers,
		Leaving:              leavers,
	})
	return err
}

func (r *DKGRunner) StartExecution() error {
	_, err := r.Client.StartExecute(context.Background(), &drand.ExecutionOptions{BeaconID: r.BeaconID})
	return err
}

func (r *DKGRunner) JoinDKG() error {
	_, err := r.Client.StartJoin(context.Background(), &drand.JoinOptions{
		BeaconID:  r.BeaconID,
		GroupFile: nil,
	})

	return err
}

func (r *DKGRunner) JoinReshare(oldGroup *key.Group) error {
	var groupFileBytes bytes.Buffer
	err := toml.NewEncoder(&groupFileBytes).Encode(oldGroup.TOML())
	if err != nil {
		return err
	}
	_, err = r.Client.StartJoin(context.Background(), &drand.JoinOptions{
		BeaconID:  r.BeaconID,
		GroupFile: groupFileBytes.Bytes(),
	})

	return err
}

func (r *DKGRunner) Accept() error {
	_, err := r.Client.StartAccept(context.Background(), &drand.AcceptOptions{
		BeaconID: r.BeaconID,
	})

	return err
}

var ErrTimeout = errors.New("DKG timed out")

func (r *DKGRunner) WaitForDKG(beaconID string, epoch uint32, numberOfSeconds int) error {
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
			panic(fmt.Sprintf("leader completed DKG in unexpected state: %s", dkg.DKGStatus(res.Complete.State).String()))
		}
		return nil
	}

	var err error
	for i := 0; i < numberOfSeconds; i++ {
		err = waitForGroupFile()
		if err == nil {
			break
		}
		fmt.Println("DKG not finished... retrying")
		time.Sleep(1 * time.Second)
	}

	return err
}
