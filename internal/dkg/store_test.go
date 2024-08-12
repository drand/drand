package dkg

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStoredDKGCanBeRetrieved(t *testing.T) {
	// create a DKG store
	store, err := NewDKGStore(t.TempDir(), nil)
	require.NoError(t, err)

	// create some DKG details
	beaconID := "myBeaconId"
	dkg := NewCompleteDKGEntry(
		t,
		beaconID,
		Executing,
		NewParticipant("somebody"),
		NewParticipant("somebody else"),
	)

	// store the DKG details
	err = store.SaveCurrent(beaconID, dkg)
	require.NoError(t, err)

	// retrieve them and ensure they're the same
	result, err := store.GetCurrent(beaconID)
	require.NoError(t, err)
	require.True(t, dkg.Equals(result))
}

func TestNoDKGStoredReturnsFresh(t *testing.T) {
	// create a DKG store
	store, err := NewDKGStore(t.TempDir(), nil)
	require.NoError(t, err)

	// fetch nothing
	beaconID := "myBeaconId"
	result, err := store.GetCurrent(beaconID)

	require.NoError(t, err)
	require.Equal(t, result, NewFreshState(beaconID))
}

func TestFetchingWrongBeaconIDReturnsFresh(t *testing.T) {
	// create a DKG store
	store, err := NewDKGStore(t.TempDir(), nil)
	require.NoError(t, err)

	// create some DKG details
	beaconID := "myBeaconId"
	dkg := NewCompleteDKGEntry(
		t,
		beaconID,
		Executing,
		NewParticipant("somebody"),
		NewParticipant("somebody else"),
	)

	// store the DKG details under one beaconId
	err = store.SaveCurrent(beaconID, dkg)
	require.NoError(t, err)

	// but try and get another beacon ID
	anotherBeaconID := "another-beacon-id"
	result, err := store.GetCurrent(anotherBeaconID)
	require.NoError(t, err)

	// you get a fresh one and not the saved one with a different ID
	require.Equal(t, result, NewFreshState(anotherBeaconID))
}

func TestNoCompletedReturnsNil(t *testing.T) {
	// create a DKG store
	store, err := NewDKGStore(t.TempDir(), nil)
	require.NoError(t, err)

	// try and get the latest finished DKG
	result, err := store.GetFinished("someBeaconId")
	require.NoError(t, err)

	require.Nil(t, result)
}

func TestGetReturnsLatestCompletedIfNoneInProgress(t *testing.T) {
	// create a DKG store
	store, err := NewDKGStore(t.TempDir(), nil)
	require.NoError(t, err)

	// create some DKG details
	beaconID := "myBeaconId"
	dkg := NewCompleteDKGEntry(
		t,
		beaconID,
		Complete,
		NewParticipant("somebody"),
		NewParticipant("somebody else"),
	)

	// store the finished DKG details
	err = store.SaveFinished(beaconID, dkg)
	require.NoError(t, err)

	// it's available using GetCurrent
	result, err := store.GetCurrent(beaconID)
	require.NoError(t, err)
	require.True(t, dkg.Equals(result))

	// and also using GetFinished
	finishedResult, err := store.GetFinished(beaconID)
	require.NoError(t, err)
	require.True(t, dkg.Equals(finishedResult))
}

func TestDeletingStateDeletesCurrentAndFinished(t *testing.T) {
	// create a DKG store
	store, err := NewDKGStore(t.TempDir(), nil)
	require.NoError(t, err)

	// create some DKG details
	beaconID := "myBeaconId"
	dkg := NewCompleteDKGEntry(
		t,
		beaconID,
		Complete,
		NewParticipant("somebody"),
		NewParticipant("somebody else"),
	)

	// store the DKG details
	err = store.SaveCurrent(beaconID, dkg)
	require.NoError(t, err)
	err = store.SaveFinished(beaconID, dkg)
	require.NoError(t, err)

	err = store.NukeState(beaconID)
	require.NoError(t, err)

	current, err := store.GetCurrent(beaconID)
	require.NoError(t, err)
	require.Equal(t, current, NewFreshState(beaconID))

	finished, err := store.GetFinished(beaconID)
	require.NoError(t, err)
	require.Nil(t, finished)
}
