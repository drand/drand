package core

import (
	"testing"
	"time"

	clock "github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/crypto"
	"github.com/drand/drand/key"
	"github.com/drand/drand/test/testlogger"
	"github.com/drand/kyber"
	"github.com/drand/kyber/util/random"
)

func TestValidateGroupTransitionGenesisTime(t *testing.T) {
	d := BeaconProcess{log: testlogger.New(t)}
	var oldgrp, newgrp key.Group

	oldgrp = key.Group{GenesisTime: 0}
	newgrp = key.Group{GenesisTime: 1}

	err := d.validateGroupTransition(&oldgrp, &newgrp)
	require.ErrorContains(t, err, "control: old and new group have different genesis time", "error validating group genesis time")
}

func TestValidateGroupTransitionPeriod(t *testing.T) {
	d := BeaconProcess{log: testlogger.New(t)}
	var oldgrp, newgrp key.Group

	oldgrp = key.Group{Period: 10}
	newgrp = key.Group{Period: 20}

	err := d.validateGroupTransition(&oldgrp, &newgrp)
	require.ErrorContains(t, err, "control: old and new group have different period", "error validating group period")
}

func TestValidateGroupTransitionBeaconID(t *testing.T) {
	d := BeaconProcess{log: testlogger.New(t)}
	var oldgrp, newgrp key.Group

	oldgrp = key.Group{ID: "beacon_test_1"}
	newgrp = key.Group{ID: "beacon_test_2"}

	err := d.validateGroupTransition(&oldgrp, &newgrp)
	require.ErrorContains(t, err, "control: old and new group have different ID", "error validating group period")
}

func TestValidateGroupTransitionGenesisSeed(t *testing.T) {
	d := BeaconProcess{log: testlogger.New(t)}
	var oldgrp, newgrp key.Group
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	randomDistPublic := func(n int) *key.DistPublic {
		publics := make([]kyber.Point, n)
		for i := range publics {
			k := sch.KeyGroup.Scalar().Pick(random.New())
			publics[i] = sch.KeyGroup.Point().Mul(k, nil)
		}
		return &key.DistPublic{Coefficients: publics}
	}

	oldgrp = key.Group{PublicKey: randomDistPublic(4)}
	newgrp = key.Group{PublicKey: randomDistPublic(3)}

	err = d.validateGroupTransition(&oldgrp, &newgrp)
	require.ErrorContains(t, err, "control: old and new group have different genesis seed", "error validating group genesis seed")
}

func TestValidateGroupTransitionTime(t *testing.T) {
	d := BeaconProcess{
		log:  testlogger.New(t),
		opts: &Config{clock: clock.NewRealClock()},
	}
	var oldgrp, newgrp key.Group

	oldgrp = key.Group{TransitionTime: time.Now().Unix()}
	newgrp = key.Group{TransitionTime: time.Now().Unix() - 1, GenesisSeed: oldgrp.GetGenesisSeed()}

	err := d.validateGroupTransition(&oldgrp, &newgrp)
	require.ErrorContains(t, err, "control: new group with transition time in the past", "error validating group period")
}
