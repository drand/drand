package dkg

import (
	bytes2 "bytes"
	"os"
	"path"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"

	pdkg "github.com/drand/drand/protobuf/dkg"

	"github.com/drand/drand/common/key"
	"github.com/drand/drand/common/log"
)

type boltStore struct {
	sync.RWMutex
	db            *bolt.DB
	log           log.Logger
	migrationLock sync.Mutex
}

const BoltFileName = "dkg.db"
const BoltStoreOpenPerm = 0660
const DirPerm = 0755

var stagedStateBucket = []byte("dkg")
var finishedStateBucket = []byte("dkg_finished")

func NewDKGStore(baseFolder string, options *bolt.Options) (Store, error) {
	err := os.MkdirAll(baseFolder, DirPerm)
	if err != nil {
		return nil, err
	}
	dbPath := path.Join(baseFolder, BoltFileName)
	db, err := bolt.Open(dbPath, BoltStoreOpenPerm, options)
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(stagedStateBucket)
		if err != nil {
			return err
		}

		_, err = tx.CreateBucketIfNotExists(finishedStateBucket)
		return err
	})
	if err != nil {
		return nil, err
	}

	store := boltStore{
		db:  db,
		log: log.New(nil, log.DebugLevel, true),
	}

	return &store, nil
}

func (s *boltStore) GetCurrent(beaconID string) (*DBState, error) {
	dkg, err := s.get(beaconID, stagedStateBucket)

	if err != nil {
		return nil, err
	}

	if dkg == nil {
		return NewFreshState(beaconID), nil
	}
	return dkg, nil
}

func (s *boltStore) GetFinished(beaconID string) (*DBState, error) {
	return s.get(beaconID, finishedStateBucket)
}

func (s *boltStore) get(beaconID string, bucketName []byte) (*DBState, error) {
	var dkg *DBState

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketName)
		if bucket == nil {
			return errors.Errorf("%s bucket was nil - this should never happen", bucketName)
		}
		value := bucket.Get([]byte(beaconID))
		if value == nil {
			return nil
		}
		t := DBStateTOML{}
		_, err := toml.NewDecoder(bytes2.NewReader(value)).Decode(&t)
		if err != nil {
			return err
		}

		d, err := t.FromTOML()
		if err != nil {
			return err
		}
		dkg = d
		return nil
	})

	return dkg, err
}

func (s *boltStore) SaveCurrent(beaconID string, state *DBState) error {
	return s.save(stagedStateBucket, beaconID, state)
}

func (s *boltStore) SaveFinished(beaconID string, state *DBState) error {
	// we want the two writes to be transactional
	// so users don't try and e.g. abort mid-completion
	// it should happen rarely, so we don't care about lock contention
	s.Lock()
	defer s.Unlock()

	// we save it to both buckets as it's the most up to date
	err := s.save(finishedStateBucket, beaconID, state)
	if err != nil {
		return err
	}

	return s.save(stagedStateBucket, beaconID, state)
}

func (s *boltStore) save(bucketName []byte, beaconID string, state *DBState) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketName)

		if bucket == nil {
			return errors.Errorf("%s bucket was nil - this should never happen", bucketName)
		}

		var bytes []byte
		b := bytes2.NewBuffer(bytes)
		err := toml.NewEncoder(b).Encode(state.TOML())
		if err != nil {
			return err
		}
		return bucket.Put([]byte(beaconID), b.Bytes())
	})
}

func (s *boltStore) Close() error {
	if err := s.db.Close(); err != nil {
		s.log.Errorw("", "boltdb", "close", "err", err)
		return err
	}

	return nil
}

func (s *boltStore) MigrateFromGroupfile(beaconID string, groupFile *key.Group, share *key.Share) error {
	if beaconID == "" {
		return errors.New("you must pass active beacon ID")
	}
	if groupFile == nil {
		return errors.New("you cannot migrate without passing active previous group file")
	}
	if share == nil {
		return errors.New("you cannot migrate without active previous distributed key share")
	}
	// we use active separate lock here to avoid reentrancy when calling `.SaveFinished()`
	s.migrationLock.Lock()
	defer s.migrationLock.Unlock()

	current, err := s.GetFinished(beaconID)
	if err != nil {
		return err
	}

	// if there has previously been active DKG in the database, abort!
	if current != nil {
		return errors.New("cannot migrate from groupfile if DKG state exists for beacon")
	}

	// map all the nodes from the group file into `drand.Participant`s
	participants := make([]*pdkg.Participant, len(groupFile.Nodes))

	if len(groupFile.Nodes) == 0 {
		return errors.New("you cannot migrate from active group file that doesn't contain node info")
	}
	for i, node := range groupFile.Nodes {
		pk, err := node.Key.MarshalBinary()
		if err != nil {
			return err
		}

		// MIGRATION PATH: the signature is `nil` here due to an incompatibility between v1 and v2 sigs over pub keys
		// the new signature will be filled in on first proposal using the new DKG
		participants[i] = &pdkg.Participant{
			Address:   node.Address(),
			Key:       pk,
			Tls:       node.TLS,
			Signature: nil,
		}
	}

	// create an epoch 1 state with the 0th node as the leader
	state := DBState{
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
	}

	return s.SaveFinished(beaconID, &state)
}
