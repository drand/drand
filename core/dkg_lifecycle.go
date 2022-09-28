package core

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"time"
)

type DKGLifecycle struct {
	log     log.Logger
	store   DKGStore
	dkgInfo map[string]*dkgInfo // beaconID to dkgInfo
}

func NewDKGLifecycle(log log.Logger, store *DKGStore) *DKGLifecycle {
	return &DKGLifecycle{
		log:     log,
		store:   *store,
		dkgInfo: make(map[string]*dkgInfo),
	}
}

func (d *DKGLifecycle) Started(beaconID string, leader *LeaderInfo, epoch uint32, timeout time.Duration) error {
	d.log.Info(fmt.Sprintf("Starting DKG with beaconID %s and epoch %d", beaconID, epoch))

	latest, err := d.store.Latest(beaconID)
	if err != nil {
		return err
	}

	if latest == nil {
		if epoch != 0 {
			return errors.New("your node has not caught up with all previous DKGs - run catchup then try again")
		}
		return d.store.Store(&DKGRecord{
			Epoch:    epoch,
			State:    Started,
			BeaconID: beaconID,
			SetupParams: &DKGSetupParams{
				Leader: leader,
			},
			Time: time.Now(),
		})
	}

	if latest.Epoch > epoch {
		return fmt.Errorf(
			"received DKG start request from a node that appears to be behind. Your current epoch: %d, their new epoch: %d",
			latest.Epoch,
			epoch,
		)
	}

	if latest.Epoch == epoch {
		if latest.State != Started {
			return fmt.Errorf("the DKG for beacon %s is already in state %s and cannot be started", beaconID, latest.State)
		}

		d.log.Infow("This DKG has already been started - ignoring DKG start command", "beaconID", beaconID)
		return nil
	}

	// this requires some kind of catchup mechanism for nodes that have left the network and rejoined
	// might be best to leave it out?
	if latest.Epoch != epoch-1 {
		return errors.New("your node has not caught up with all previous DKGs - run catchup then try again")
	}

	if !terminalState(latest.State) {
		return fmt.Errorf("cannot start a new DKG - one is already in progress with state %s", latest.State.String())
	}

	now := time.Now()
	return d.store.Store(&DKGRecord{
		Epoch:    epoch,
		State:    Started,
		BeaconID: beaconID,
		Time:     now,
		SetupParams: &DKGSetupParams{
			Leader:  leader,
			Timeout: now.Add(timeout),
		},
	})
}

func (d *DKGLifecycle) Ready(beaconID string, epoch uint32, oldGroup *key.Group, newGroup *key.Group) error {
	d.log.Info(fmt.Sprintf("DKG moving to messaging phase, beacon ID %s", beaconID))
	latest, err := latestDKGFromEpoch(d.store, beaconID, epoch)
	if err != nil {
		return err
	}
	if latest == nil {
		return fmt.Errorf("cannot move to DKG ready phase as there is no DKG in progress.  Current epoch %d", epoch)
	}
	if latest.State == Executing {
		d.log.Info("DKG already in executing state - ignoring DKG ready command", "beaconID", beaconID)
		return nil
	}
	if latest.State == Finished {
		return errors.New("cannot move to DKG ready phase as there is no DKG in progress")
	}

	// the `key.Group` is serialised as binary right now because the `kyber.Point`s don't play so nicely with
	// the toml or json enc/dec
	var oldGroupBuf bytes.Buffer
	if oldGroup != nil {
		err = toml.NewEncoder(&oldGroupBuf).Encode(oldGroup.TOML())
		if err != nil {
			return err
		}
	}
	var newGroupBuf bytes.Buffer
	if newGroup != nil {
		err = toml.NewEncoder(&newGroupBuf).Encode(newGroup.TOML())
		if err != nil {
			return err
		}
	}
	return d.store.Store(&DKGRecord{
		Epoch:       latest.Epoch,
		State:       Executing,
		BeaconID:    beaconID,
		Time:        time.Now(),
		SetupParams: latest.SetupParams,
		ExecutionParams: &DKGExecutionParams{
			OldGroup: oldGroupBuf.String(),
			NewGroup: newGroupBuf.String(),
		},
	})
}

func (d *DKGLifecycle) Finished(beaconID string, epoch uint32, finalGroup *key.Group) error {
	d.log.Infow("Finishing DKG", "beaconID", beaconID)
	latest, err := latestDKGFromEpoch(d.store, beaconID, epoch)
	if err != nil {
		return err
	}
	if latest == nil {
		return errors.New("there was no DKG in progress to finish")
	}
	if latest.State == Finished {
		d.log.Infow("DKG already in Finished state - ignoring DKG finish command", "beaconID", beaconID)
		return nil
	}
	if latest.State != Executing {
		return fmt.Errorf("expected a started DKG to finish, got %s", latest.State.String())
	}

	var finalGroupBuf bytes.Buffer
	if finalGroup != nil {
		err = toml.NewEncoder(&finalGroupBuf).Encode(finalGroup.TOML())
		if err != nil {
			return err
		}
	}
	return d.store.Store(&DKGRecord{
		State:           Finished,
		BeaconID:        beaconID,
		Time:            time.Now(),
		SetupParams:     latest.SetupParams,
		ExecutionParams: latest.ExecutionParams,
		CompletionParams: &DKGCompletionParams{
			Group: finalGroupBuf.String(),
		},
	})
}

// returns the latest DKG if its epoch matches
// returns nil if there are no DKGs
// returns an error if there is a DKG but its epoch is wrong
func latestDKGFromEpoch(store DKGStore, beaconID string, epoch uint32) (*DKGRecord, error) {
	latest, err := store.Latest(beaconID)

	if err != nil {
		return nil, err
	}

	if latest == nil {
		return nil, nil
	}

	if latest.Epoch < epoch {
		return nil, fmt.Errorf("DKG appears to be behind. Your epoch: %d, the epoch of your peer: %d", latest.Epoch, epoch)
	}

	if latest.Epoch > epoch {
		return nil, fmt.Errorf("received DKG requests from a node that appears to be behind. Your epoch: %d, their epoch: %d", latest.Epoch, epoch)
	}

	return latest, nil
}

type DKGStatus int

func (d DKGStatus) String() string {
	switch d {
	case Error:
		return "Error"
	case NoneStarted:
		return "NoneStarted"
	case InProgress:
		return "InProgress"
	case Idle:
		return "Idle"
	}
	return "invalid status"
}

const (
	Error DKGStatus = iota
	NoneStarted
	InProgress
	Idle
)

func (d *DKGLifecycle) Status(beaconID string) (DKGStatus, error) {
	latest, err := d.store.Latest(beaconID)
	if err != nil {
		return Error, err
	}

	if latest == nil {
		return NoneStarted, nil
	}

	switch latest.State {
	case Started:
		return InProgress, nil
	case Finished:
		return Idle, nil
	}

	return Error, errors.New("DKG status has reached some undefined behaviour")
}
