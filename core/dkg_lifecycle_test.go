package core

import (
	"errors"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDKGStarted(t *testing.T) {
	tests := []struct {
		name                    string
		epoch                   uint32
		storeDkg                *DKGRecord
		storeError              error
		shouldSkipDatabaseWrite bool
		shouldError             bool
		expectedError           error
	}{
		{
			name:        "empty DKG history fails with non-zero epoch",
			epoch:       1,
			storeDkg:    nil,
			shouldError: true,
		},
		{
			name:        "empty DKG history succeeds with zero epoch",
			epoch:       0,
			storeDkg:    nil,
			shouldError: false,
		},
		{
			name:  "latest DKG higher epoch as start fails with error",
			epoch: 1,
			storeDkg: &DKGRecord{
				Epoch:    2,
				BeaconID: "default",
				State:    Finished,
				SetupParams: &DKGSetupParams{
					Leader: nil,
				},
				Time: time.Now(),
			},
			shouldError: true,
		},
		{
			name:  "latest DKG same epoch as start but non-started state fails with error",
			epoch: 1,
			storeDkg: &DKGRecord{
				Epoch:    1,
				BeaconID: "default",
				State:    Finished,
				SetupParams: &DKGSetupParams{
					Leader: nil,
				},
				Time: time.Now(),
			},
			shouldError: true,
		},
		{
			name:  "latest DKG same epoch as start and started state succeeds but doesn't save to DB",
			epoch: 1,
			storeDkg: &DKGRecord{
				Epoch:    1,
				BeaconID: "default",
				State:    Started,
				SetupParams: &DKGSetupParams{
					Leader: nil,
				}, Time: time.Now(),
			},
			shouldError:             false,
			shouldSkipDatabaseWrite: true,
		},
		{
			name:  "latest DKG not finished fails with error",
			epoch: 2,
			storeDkg: &DKGRecord{
				Epoch:    1,
				BeaconID: "default",
				State:    Executing,
				SetupParams: &DKGSetupParams{
					Leader: nil,
				}, Time: time.Now(),
			},
			shouldError: true,
		},
		{
			name:          "store errors are propagated",
			epoch:         1,
			storeDkg:      nil,
			storeError:    errors.New("some error"),
			shouldError:   true,
			expectedError: errors.New("some error"),
		},
	}

	for _, test := range tests {
		beaconID := "default"
		mockStore := MockStore{}
		lifecycle := DKGLifecycle{
			log:   log.DefaultLogger(),
			store: &mockStore,
		}

		t.Run(test.name, func(t *testing.T) {
			mockStore.On("Latest", beaconID).Return(test.storeDkg, test.storeError)
			mockStore.On("Store", mock.Anything).Return(nil)

			err := lifecycle.Started(beaconID, nil, test.epoch, time.Minute*5)

			if test.shouldError {
				require.Error(t, err)
				mockStore.AssertNotCalled(t, "Store", mock.Anything)
			} else {
				if !test.shouldSkipDatabaseWrite {
					mockStore.AssertCalled(t, "Store", mock.Anything)
				}
				require.NoError(t, err)
			}
		})
	}
}

func TestDKGReady(t *testing.T) {
	tests := []struct {
		name                    string
		epoch                   uint32
		storeDkg                *DKGRecord
		storeError              error
		shouldSkipDatabaseWrite bool
		shouldError             bool
		expectedError           error
	}{
		{
			name:          "store errors are propagated",
			epoch:         1,
			storeDkg:      nil,
			storeError:    errors.New("some insane error"),
			expectedError: errors.New("some insane error"),
			shouldError:   true,
		},
		{
			name:  "latest DKG of earlier epoch fails",
			epoch: 2,
			storeDkg: &DKGRecord{
				Epoch:    1,
				BeaconID: "default",
				State:    Started,
				SetupParams: &DKGSetupParams{
					Leader: nil,
				}, Time: time.Now(),
			},
			shouldError: true,
		},
		{
			name:  "latest DKG of newer epoch fails",
			epoch: 1,
			storeDkg: &DKGRecord{
				Epoch:    2,
				BeaconID: "default",
				State:    Started,
				SetupParams: &DKGSetupParams{
					Leader: nil,
				}, Time: time.Now(),
			},
			shouldError: true,
		},
		{
			name:  "no DKG in progress fails",
			epoch: 2,
			storeDkg: &DKGRecord{
				Epoch:    1,
				BeaconID: "default",
				State:    Finished,
				SetupParams: &DKGSetupParams{
					Leader: nil,
				}, Time: time.Now(),
			},
			shouldError: true,
		},
		{
			name:  "latest DKG already finished fails with error",
			epoch: 2,
			storeDkg: &DKGRecord{
				Epoch:    2,
				BeaconID: "default",
				State:    Finished,
				SetupParams: &DKGSetupParams{
					Leader: nil,
				}, Time: time.Now(),
			},
			shouldError: true,
		},
		{
			name:  "latest DKG already executing succeeds but does not call the store",
			epoch: 2,
			storeDkg: &DKGRecord{
				Epoch:    2,
				BeaconID: "default",
				State:    Executing,
				SetupParams: &DKGSetupParams{
					Leader: nil,
				}, Time: time.Now(),
			},
			shouldError:             false,
			shouldSkipDatabaseWrite: true,
		},
		{
			name:  "latest DKG started stores a new executing DKG state",
			epoch: 2,
			storeDkg: &DKGRecord{
				Epoch:    2,
				BeaconID: "default",
				State:    Started,
				SetupParams: &DKGSetupParams{
					Leader: nil,
				}, Time: time.Now(),
			},
			shouldError: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beaconID := "default"
			mockStore := MockStore{}
			lifecycle := DKGLifecycle{
				log:   log.DefaultLogger(),
				store: &mockStore,
			}

			mockStore.On("Latest", beaconID).Return(test.storeDkg, test.storeError)
			mockStore.On("Store", mock.Anything).Return(nil)

			err := lifecycle.Ready(beaconID, test.epoch, nil, nil)

			if test.shouldError {
				require.Error(t, err)
				mockStore.AssertNotCalled(t, "Store", mock.Anything)
			} else {
				if !test.shouldSkipDatabaseWrite {
					mockStore.AssertCalled(t, "Store", mock.Anything)
				}
				require.NoError(t, err)
			}
		})
	}
}

func TestDKGFinished(t *testing.T) {
	tests := []struct {
		name                    string
		epoch                   uint32
		storeDkg                *DKGRecord
		storeError              error
		shouldSkipDatabaseWrite bool
		shouldError             bool
		expectedError           error
	}{
		{
			name:          "store errors are propagated",
			epoch:         1,
			storeDkg:      nil,
			storeError:    errors.New("oh no"),
			expectedError: errors.New("oh no"),
			shouldError:   true,
		},
		{
			name:        "no latest DKG fails",
			epoch:       1,
			storeDkg:    nil,
			shouldError: true,
		},
		{
			name:  "latest DKG of different epoch fails",
			epoch: 1,
			storeDkg: &DKGRecord{
				Epoch:    2,
				BeaconID: "default",
				State:    Executing,
				SetupParams: &DKGSetupParams{
					Leader: nil,
				}, Time: time.Now(),
			},
			shouldError: true,
		},
		{
			name:  "latest DKG already finished succeeds but doesn't call the store",
			epoch: 1,
			storeDkg: &DKGRecord{
				Epoch:    1,
				BeaconID: "default",
				State:    Finished,
				SetupParams: &DKGSetupParams{
					Leader: nil,
				}, Time: time.Now(),
			},
			shouldError:             false,
			shouldSkipDatabaseWrite: true,
		},
		{
			name:  "latest DKG not executing or already finished fails",
			epoch: 1,
			storeDkg: &DKGRecord{
				Epoch:    1,
				BeaconID: "default",
				State:    Started,
				SetupParams: &DKGSetupParams{
					Leader: nil,
				}, Time: time.Now(),
			},
			shouldError: true,
		},
		{
			name:  "latest DKG in executing state and correct epoch succeeds",
			epoch: 1,
			storeDkg: &DKGRecord{
				Epoch:    1,
				BeaconID: "default",
				State:    Executing,
				SetupParams: &DKGSetupParams{
					Leader: nil,
				}, Time: time.Now(),
			},
			shouldError: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beaconID := "default"
			mockStore := MockStore{}
			lifecycle := DKGLifecycle{
				log:   log.DefaultLogger(),
				store: &mockStore,
			}

			mockStore.On("Latest", beaconID).Return(test.storeDkg, test.storeError)
			mockStore.On("Store", mock.Anything).Return(nil)

			err := lifecycle.Finished(beaconID, test.epoch, &key.Group{})

			if test.shouldError {
				require.Error(t, err)
				mockStore.AssertNotCalled(t, "Store", mock.Anything)
			} else {
				if !test.shouldSkipDatabaseWrite {
					mockStore.AssertCalled(t, "Store", mock.Anything)
				}
				require.NoError(t, err)
			}
		})
	}
}

type MockStore struct {
	mock.Mock
	DKGStore
}

func (s *MockStore) Latest(beaconID string) (*DKGRecord, error) {
	args := s.Called(beaconID)
	return args.Get(0).(*DKGRecord), args.Error(1)
}

func (s *MockStore) Store(record *DKGRecord) error {
	args := s.Called(record)
	return args.Error(0)
}
