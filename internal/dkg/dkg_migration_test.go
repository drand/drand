package dkg

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/share/dkg"
)

func TestNilGroupFails(t *testing.T) {
	store, err := NewDKGStore(t.TempDir(), nil)
	require.NoError(t, err)

	err = store.MigrateFromGroupfile("some-beacon", nil, fakeShare())
	require.Error(t, err)
}

func TestNilKeyShareFails(t *testing.T) {
	store, err := NewDKGStore(t.TempDir(), nil)
	require.NoError(t, err)

	group := fakeGroup()

	err = store.MigrateFromGroupfile("some-beacon", group, nil)
	require.Error(t, err)
}

func TestEmptyBeaconIDFails(t *testing.T) {
	store, err := NewDKGStore(t.TempDir(), nil)
	require.NoError(t, err)

	keyShare := fakeShare()
	group := fakeGroup()

	err = store.MigrateFromGroupfile("", group, keyShare)
	require.Error(t, err)
}

func TestStateAlreadyInDBForBeaconIDFails(t *testing.T) {
	sch, _ := crypto.GetSchemeFromEnv()

	// create a new store
	beaconID := "banana"
	store, err := NewDKGStore(t.TempDir(), nil)
	require.NoError(t, err)

	// save an existing state in it
	now := time.Now()
	err = store.SaveFinished(beaconID, &DBState{
		BeaconID:      beaconID,
		Epoch:         1,
		State:         Complete,
		Threshold:     1,
		Timeout:       now,
		SchemeID:      sch.Name,
		GenesisTime:   now,
		GenesisSeed:   []byte("deadbeef"),
		CatchupPeriod: 1,
		BeaconPeriod:  3,
		Leader:        nil,
		Remaining:     nil,
		Joining:       nil,
		Leaving:       nil,
		Acceptors:     nil,
		Rejectors:     nil,
		FinalGroup:    nil,
		KeyShare:      nil,
	})
	require.NoError(t, err)

	err = store.MigrateFromGroupfile(beaconID, fakeGroup(), fakeShare())
	require.Error(t, err)
}

func TestStateInDBForDifferentBeaconIDDoesntFail(t *testing.T) {
	sch, _ := crypto.GetSchemeFromEnv()
	// create a new store
	beaconID := "banana"
	aDifferentBeaconID := "different-beacon-id"
	store, err := NewDKGStore(t.TempDir(), nil)
	require.NoError(t, err)

	// save an existing state but for a different beacon ID
	now := time.Now()
	err = store.SaveFinished(aDifferentBeaconID, &DBState{
		BeaconID:      aDifferentBeaconID,
		Epoch:         1,
		State:         Complete,
		Threshold:     1,
		Timeout:       now,
		SchemeID:      sch.Name,
		GenesisTime:   now,
		GenesisSeed:   []byte("deadbeef"),
		CatchupPeriod: 1,
		BeaconPeriod:  3,
		Leader:        nil,
		Remaining:     nil,
		Joining:       nil,
		Leaving:       nil,
		Acceptors:     nil,
		Rejectors:     nil,
		FinalGroup:    nil,
		KeyShare:      nil,
	})
	require.NoError(t, err)

	err = store.MigrateFromGroupfile(beaconID, fakeGroup(), fakeShare())
	require.NoError(t, err)
}

func TestValidMigrationIsRetrievable(t *testing.T) {
	// create a new store
	beaconID := "banana"
	store, err := NewDKGStore(t.TempDir(), nil)
	require.NoError(t, err)

	// perform the migration
	err = store.MigrateFromGroupfile(beaconID, fakeGroup(), fakeShare())
	require.NoError(t, err)

	// get the finished migrated state and check some of its fields
	state, err := store.GetFinished(beaconID)
	require.NoError(t, err)
	require.Equal(t, state.BeaconID, beaconID)
	require.Equal(t, state.State, Complete)
}

func TestInvalidMigrationIsNotRetrievable(t *testing.T) {
	// create a new store
	beaconID := "banana"
	store, err := NewDKGStore(t.TempDir(), nil)
	require.NoError(t, err)

	// perform an invalid migration
	err = store.MigrateFromGroupfile(beaconID, nil, nil)
	require.Error(t, err)

	// get the finished migrated state and check some of its fields
	state, err := store.GetFinished(beaconID)
	require.NoError(t, err)
	require.Nil(t, state)
}

func fakeShare() *key.Share {
	sch, _ := crypto.GetSchemeFromEnv()
	scalarOne := sch.KeyGroup.Scalar().One()
	s := &share.PriShare{I: 2, V: scalarOne}
	return &key.Share{DistKeyShare: dkg.DistKeyShare{Share: s}, Scheme: sch}
}

func fakeGroup() *key.Group {
	sch, _ := crypto.GetSchemeFromEnv()
	return &key.Group{
		Threshold:     1,
		Period:        3,
		Scheme:        sch,
		ID:            "default",
		CatchupPeriod: 2,
		Nodes: []*key.Node{{
			Index: 0,
			Identity: &key.Identity{
				Key:       sch.KeyGroup.Point(),
				Addr:      "localhost:1234",
				Signature: []byte("abcd1234"),
				Scheme:    sch,
			},
		}},
		GenesisTime:    time.Now().Unix(),
		GenesisSeed:    []byte("deadbeef"),
		TransitionTime: time.Now().Unix(),
		PublicKey:      fakePublic(),
	}
}

func fakePublic() *key.DistPublic {
	sch, _ := crypto.GetSchemeFromEnv()
	return &key.DistPublic{Coefficients: []kyber.Point{sch.KeyGroup.Point()}}
}
