package dkg

import (
	bytes2 "bytes"
	"os"
	"path"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	pdkg "github.com/drand/drand/v2/protobuf/dkg"

	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/common/log"
)

const FileName = "dkg.toml"
const StagedFileName = "dkg.staged.toml"

const DirPerm = 0755

type FileStore struct {
	baseFolder string
	log        log.Logger
}

func NewDKGStore(baseFolder string, logLevel int) (*FileStore, error) {
	err := os.MkdirAll(baseFolder, DirPerm)
	if err != nil {
		return nil, err
	}
	return &FileStore{
		baseFolder: baseFolder,
		log:        log.New(nil, logLevel, true),
	}, nil
}

func getFromFilePath(path string) (*DBState, error) {
	t := DBStateTOML{}
	_, err := toml.DecodeFile(path, &t)
	if err != nil {
		return nil, err
	}
	state, err := t.FromTOML()
	if err != nil {
		return nil, err
	}
	return state, nil
}

func (fs FileStore) GetCurrent(beaconID string) (*DBState, error) {
	f, err := getFromFilePath(path.Join(fs.baseFolder, beaconID, StagedFileName))
	if errors.Is(err, os.ErrNotExist) {
		fs.log.Debug("No DKG file found, returning new state")
		return NewFreshState(beaconID), nil
	}
	return f, err
}

func (fs FileStore) GetFinished(beaconID string) (*DBState, error) {
	f, err := getFromFilePath(path.Join(fs.baseFolder, beaconID, FileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return f, err
}

func saveTOMLToFilePath(filepath string, state *DBState) error {
	w, err := os.Create(filepath)
	if err != nil {
		return err
	}
	t := state.TOML()
	err = toml.NewEncoder(w).Encode(&t)
	if err != nil {
		return err
	}
	return w.Close()
}

// SaveCurrent stores a DKG packet for an ongoing DKG
func (fs FileStore) SaveCurrent(beaconID string, state *DBState) error {
	return saveTOMLToFilePath(path.Join(fs.baseFolder, beaconID, StagedFileName), state)
}

// SaveFinished stores a completed, successful DKG and overwrites the current packet
func (fs FileStore) SaveFinished(beaconID string, state *DBState) error {
	return saveTOMLToFilePath(path.Join(fs.baseFolder, beaconID, FileName), state)
}

func (fs FileStore) Close() error {
	// Nothing to do for flat-file management
	return nil
}

func (fs FileStore) MigrateFromGroupfile(beaconID string, groupFile *key.Group, share *key.Share) error {
	fs.log.Debug("Converting group file for beaconID %s ...", beaconID)
	if beaconID == "" {
		return errors.New("you must pass a beacon ID")
	}
	if groupFile == nil {
		return errors.New("you cannot migrate without passing a previous group file")
	}
	if share == nil {
		return errors.New("you cannot migrate without a previous distributed key share")
	}

	dbState, err := GroupFileToDBState(beaconID, groupFile, share)
	if err != nil {
		return err
	}

	dkgFilePath := path.Join(fs.baseFolder, beaconID, FileName)
	fs.log.Debug("Writing DKG file %s for for beaconID %s ...", dkgFilePath, beaconID)
	if err = saveTOMLToFilePath(dkgFilePath, dbState); err != nil {
		return err
	}
	stagedDkgFilePath := path.Join(fs.baseFolder, beaconID, StagedFileName)
	fs.log.Debug("Writing DKG file %s for for beaconID %s ...", stagedDkgFilePath, beaconID)
	return saveTOMLToFilePath(stagedDkgFilePath, dbState)
}

func encodeState(state *DBState) ([]byte, error) {
	var bytes []byte
	b := bytes2.NewBuffer(bytes)
	err := toml.NewEncoder(b).Encode(state.TOML())
	if err != nil {
		return nil, err
	}
	return b.Bytes(), err
}

func GroupFileToDBState(beaconID string, groupFile *key.Group, share *key.Share) (*DBState, error) {
	// map all the nodes from the group file into `drand.Participant`s
	participants := make([]*pdkg.Participant, len(groupFile.Nodes))

	if len(groupFile.Nodes) == 0 {
		return nil, errors.New("you cannot migrate from a group file that doesn't contain node info")
	}
	for i, node := range groupFile.Nodes {
		pk, err := node.Key.MarshalBinary()
		if err != nil {
			return nil, err
		}

		// MIGRATION PATH: the signature is `nil` here due to an incompatibility between v1 and v2 sigs over pub keys
		// the new signature will be filled in on first proposal using the new DKG
		participants[i] = &pdkg.Participant{
			Address:   node.Address(),
			Key:       pk,
			Signature: nil,
		}
	}

	// create an epoch 1 state with the 0th node as the leader
	return &DBState{
		BeaconID:      beaconID,
		Epoch:         1,
		State:         Complete,
		Threshold:     uint32(groupFile.Threshold),
		Timeout:       time.Now(),
		SchemeID:      groupFile.Scheme.Name,
		GenesisTime:   time.Unix(groupFile.GenesisTime, 0),
		GenesisSeed:   groupFile.GenesisSeed,
		CatchupPeriod: groupFile.CatchupPeriod,
		BeaconPeriod:  groupFile.Period,
		Leader:        participants[0],
		Remaining:     nil,
		Joining:       participants,
		Leaving:       nil,
		Acceptors:     participants,
		Rejectors:     nil,
		FinalGroup:    groupFile,
		KeyShare:      share,
	}, nil

}

// NukeState deletes the directory corresponding to the specified beaconID
func (fs FileStore) NukeState(beaconID string) error {
	return os.RemoveAll(path.Join(fs.baseFolder, beaconID))
}
