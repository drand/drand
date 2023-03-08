package dkg

import (
	"fmt"

	"github.com/drand/drand/protobuf/drand"
)

// executeAction fetches the latest DKG state, applies the action to it and stores it back in the database
func (d *DKGProcess) executeAction(
	name string,
	beaconID string,
	action func(me *drand.Participant, current *DBState) (*DBState, error),
) error {
	return d.executeActionWithCallback(name, beaconID, action, nil)
}

// executeActionWithCallback fetches the latest DKG state, applies the action to it, passes that new state
// to a callback then stores the new state in the database if the callback was successful
func (d *DKGProcess) executeActionWithCallback(
	name string,
	beaconID string,
	createNewState func(me *drand.Participant, current *DBState) (*DBState, error),
	callback func(me *drand.Participant, newState *DBState) error,
) error {
	var err error
	d.log.Infow(fmt.Sprintf("Processing %s", name), "beaconID", beaconID)

	defer func() {
		if err != nil {
			d.log.Errorw(fmt.Sprintf("Error processing %s", name), "beaconID", beaconID, "error", err)
		} else {
			d.log.Infow(fmt.Sprintf("%s successful", name), "beaconID", beaconID)
		}
	}()

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return err
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return err
	}

	nextState, err := createNewState(me, current)
	if err != nil {
		return err
	}

	if callback != nil {
		err = callback(me, nextState)
		if err != nil {
			return err
		}
	}
	err = d.store.SaveCurrent(beaconID, nextState)
	return err
}
