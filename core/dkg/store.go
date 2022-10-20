package dkg

import (
	bytes2 "bytes"
	"path"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/drand/drand/log"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

type boltStore struct {
	sync.RWMutex
	db  *bolt.DB
	log log.Logger
}

const BoltFileName = "dkg.db"
const BoltStoreOpenPerm = 0660

var stagedStateBucket = []byte("dkg")
var finishedStateBucket = []byte("dkg_finished")

func NewDKGStore(baseFolder string, options *bolt.Options) (Store, error) {
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
		log: log.NewLogger(nil, log.LogDebug),
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
		log.DefaultLogger().Errorw("", "boltdb", "close", "err", err)
		return err
	}

	return nil
}
