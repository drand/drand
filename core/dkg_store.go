package core

import (
	"github.com/drand/drand/log"
	json "github.com/nikkolasg/hexjson"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
	"path"
	"sync"
)

type boltStore struct {
	sync.RWMutex
	db  *bolt.DB
	log log.Logger
}

const BoltFileName = "dkg.db"
const BoltStoreOpenPerm = 0660

var DKGStateBucket = []byte("dkg")
var DKGFinishedStateBucket = []byte("dkg_finished")

func NewDKGStore(baseFolder string, options *bolt.Options) (DKGStore, error) {
	dbPath := path.Join(baseFolder, BoltFileName)
	db, err := bolt.Open(dbPath, BoltStoreOpenPerm, options)
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(DKGStateBucket)
		if err != nil {
			return err
		}

		_, err = tx.CreateBucketIfNotExists(DKGFinishedStateBucket)
		return err
	})

	store := boltStore{
		db:  db,
		log: log.NewLogger(nil, log.LogDebug),
	}

	return &store, nil
}

func (s *boltStore) GetCurrent(beaconID string) (*DKGDetails, error) {
	dkg, err := s.get(beaconID, DKGStateBucket)

	if err != nil {
		return nil, err
	}

	if dkg == nil {
		return NewFreshState(beaconID), nil
	}
	return dkg, nil
}

func (s *boltStore) GetFinished(beaconID string) (*DKGDetails, error) {
	return s.get(beaconID, DKGFinishedStateBucket)
}

func (s *boltStore) get(beaconID string, bucketName []byte) (*DKGDetails, error) {
	var dkg *DKGDetails

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketName)
		if bucket == nil {
			return errors.Errorf("%s bucket was nil - this should never happen", bucketName)
		}
		value := bucket.Get([]byte(beaconID))
		if value == nil {
			return nil
		}
		return json.Unmarshal(value, &dkg)
	})

	return dkg, err
}

func (s *boltStore) SaveCurrent(beaconID string, state *DKGDetails) error {
	return s.save(DKGStateBucket, beaconID, state)
}

func (s *boltStore) SaveFinished(beaconID string, state *DKGDetails) error {
	// we want this to be transactional and should lock out other accesses
	// so users don't try and e.g. abort mid-completion
	// it should happen rarely, so we don't care about lock contention
	s.Lock()
	defer s.Unlock()

	// we save it to both buckets as it's the most up to date
	err := s.save(DKGFinishedStateBucket, beaconID, state)
	if err != nil {
		return err
	}

	return s.save(DKGStateBucket, beaconID, state)
}

func (s *boltStore) save(bucketName []byte, beaconID string, state *DKGDetails) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketName)

		if bucket == nil {
			return errors.Errorf("%s bucket was nil - this should never happen", bucketName)
		}

		bytes, err := json.Marshal(state)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(beaconID), bytes)
	})
}

func (s *boltStore) Close() error {
	if err := s.db.Close(); err != nil {
		log.DefaultLogger().Errorw("", "boltdb", "close", "err", err)
		return err
	}

	return nil
}
