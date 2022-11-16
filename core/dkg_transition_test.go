package core

import (
	"errors"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExecutingDKGTransition(t *testing.T) {
	// set up some dependencies
	store := new(StoreMock)

	process := DKGProcess{
		fetchIdentityForBeacon: noOpFetchIdentity,
		store:                  store,
		log:                    log.NewLogger(nil, log.LogDebug),
	}

	// create a stub mapping where everything successful
	beaconID := "some-beacon"
	mapping := stubMapping{
		enrichResult: "stub",
		enrichError:  nil,
		applyResult:  NewFreshState(beaconID),
		applyError:   nil,
	}

	// database gets and saves successfully
	store.On("GetCurrent", beaconID).Return(NewFreshState(beaconID), nil)
	store.On("SaveCurrent", beaconID, NewFreshState(beaconID)).Return(nil)

	// state transition works
	err := executeProtocolSteps[string, string, string](&process, beaconID, mapping, "some-input")
	require.NoError(t, err)
}

func TestCompleteDKGSavedAsFinished(t *testing.T) {
	// create some dependencies
	store := new(StoreMock)
	process := DKGProcess{
		fetchIdentityForBeacon: noOpFetchIdentity,
		store:                  store,
		log:                    log.NewLogger(nil, log.LogDebug),
	}

	// create a stub mapping where everything successful
	beaconID := "some-beacon"
	finishedDKG := NewFullDKGEntry(beaconID, Complete, nil)
	mapping := stubMapping{
		enrichResult: "stub",
		enrichError:  nil,
		applyResult:  finishedDKG,
		applyError:   nil,
	}

	// database gets and saves as Finished successfully
	store.On("GetCurrent", beaconID).Return(NewFreshState(beaconID), nil)
	store.On("SaveFinished", beaconID, finishedDKG).Return(nil)

	// state transition works
	err := executeProtocolSteps[string, string, string](&process, beaconID, mapping, "some-input")
	require.NoError(t, err)
}

func TestStateTransitionPropagatesErrorsInInitialMapping(t *testing.T) {
	// create some dependencies
	store := new(StoreMock)
	process := DKGProcess{
		fetchIdentityForBeacon: noOpFetchIdentity,
		store:                  store,
		log:                    log.NewLogger(nil, log.LogDebug),
	}

	// error during enrichment of the packet
	expectedError := InvalidPacket
	mapping := stubMapping{
		enrichResult: "",
		enrichError:  expectedError,
		applyResult:  nil,
		applyError:   UnexpectedError,
	}

	// expect the error created above
	err := executeProtocolSteps[string, string, string](&process, "some-beacon", mapping, "some-input")
	require.Equal(t, expectedError, err)
}

func TestStateTransitionPropagatesErrorInDKGMapping(t *testing.T) {
	// create some dependencies
	store := new(StoreMock)
	process := DKGProcess{
		fetchIdentityForBeacon: noOpFetchIdentity,
		store:                  store,
		log:                    log.NewLogger(nil, log.LogDebug),
	}

	// error during DKG state application of the packet
	beaconID := "some-beacon"
	expectedError := InvalidEpoch
	mapping := stubMapping{
		enrichResult: "great",
		enrichError:  nil,
		applyResult:  nil,
		applyError:   expectedError,
	}

	store.On("GetCurrent", beaconID).Return(NewFreshState(beaconID), nil)

	// expect the error created above
	err := executeProtocolSteps[string, string, string](&process, "some-beacon", mapping, "some-input")
	require.Error(t, err)
}

func TestDatabaseGetErrorPropagated(t *testing.T) {
	// create some dependencies
	store := new(StoreMock)
	process := DKGProcess{
		fetchIdentityForBeacon: noOpFetchIdentity,
		store:                  store,
		log:                    log.NewLogger(nil, log.LogDebug),
	}

	// valid looking DKG transition
	beaconID := "some-beacon"
	mapping := stubMapping{
		enrichResult: "great",
		enrichError:  nil,
		applyResult:  NewFullDKGEntry(beaconID, Executing, nil),
		applyError:   nil,
	}

	expectedError := errors.New("database blew up")
	store.On("GetCurrent", beaconID).Return(nil, expectedError)

	// expect the error created above
	err := executeProtocolSteps[string, string, string](&process, "some-beacon", mapping, "some-input")
	require.Equal(t, expectedError, err)
}

func TestDatabaseSaveErrorPropagated(t *testing.T) {
	// create some dependencies
	store := new(StoreMock)
	process := DKGProcess{
		fetchIdentityForBeacon: noOpFetchIdentity,
		store:                  store,
		log:                    log.NewLogger(nil, log.LogDebug),
	}

	// valid looking DKG transition
	beaconID := "some-beacon"
	dkgState := NewFullDKGEntry(beaconID, Executing, nil)
	mapping := stubMapping{
		enrichResult: "great",
		enrichError:  nil,
		applyResult:  dkgState,
		applyError:   nil,
	}

	expectedError := errors.New("database blew up")
	store.On("GetCurrent", beaconID).Return(dkgState, nil)
	store.On("SaveCurrent", beaconID, dkgState).Return(expectedError)

	// expect the error created above
	err := executeProtocolSteps[string, string, string](&process, "some-beacon", mapping, "some-input")
	require.Equal(t, expectedError, err)
}

type stubMapping struct {
	enrichResult  string
	enrichError   error
	applyResult   *DKGDetails
	applyError    error
	responses     []*NetworkRequest[string]
	responseError error
	forwardError  error
}

func (s stubMapping) Enrich(_ string) (string, error) {
	return s.enrichResult, s.enrichError
}

func (s stubMapping) Apply(_ string, _ *DKGDetails) (*DKGDetails, error) {
	return s.applyResult, s.applyError
}

func (s stubMapping) Requests(_ string, _ *DKGDetails) ([]*NetworkRequest[string], error) {
	return s.responses, s.responseError
}

func (s stubMapping) ForwardRequest(_ drand.DKGClient, _ *NetworkRequest[string]) error {
	return s.forwardError
}

type StoreMock struct {
	mock.Mock
	DKGStore
}

// depending on the state of the world
func (s StoreMock) GetCurrent(beaconID string) (*DKGDetails, error) {
	args := s.MethodCalled("GetCurrent", beaconID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DKGDetails), args.Error(1)
}

// GetFinished returns the last completed DKG state (i.e. completed or aborted), or nil if one has not been finished
func (s StoreMock) GetFinished(beaconID string) (*DKGDetails, error) {
	args := s.MethodCalled("GetFinished", beaconID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DKGDetails), args.Error(1)
}

// SaveCurrent stores a DKG packet for an ongoing DKG
func (s StoreMock) SaveCurrent(beaconID string, state *DKGDetails) error {
	args := s.MethodCalled("SaveCurrent", beaconID, state)
	return args.Error(0)
}

// SaveFinished stores a completed, successful DKG and overwrites the current packet
func (s StoreMock) SaveFinished(beaconID string, state *DKGDetails) error {
	args := s.MethodCalled("SaveFinished", beaconID, state)
	return args.Error(0)
}

// Close closes and cleans up any database handles
func (s StoreMock) Close() error {
	s.Called()
	return nil
}

func noOpFetchIdentity(_ string) (*key.Identity, error) {
	return nil, nil
}
