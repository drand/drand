package core

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"github.com/drand/drand/common"
	"github.com/drand/drand/fs"
	bolt "go.etcd.io/bbolt"
	"path"
	"sync"
	"time"
)

type DKGStore interface {
	Store(record *DKGRecord) error
	Latest(beaconID string) (*DKGRecord, error)
	History(beaconID string) (*[]DKGRecord, error)
}

type DKGState int

const (
	Started   DKGState = iota // waiting for others to join
	Executing                 // leader has moved to the response/justif phase
	Finished                  // group file has been emitted
	Cancelled
)

func terminalState(state DKGState) bool {
	return state == Finished || state == Cancelled
}

func (s DKGState) String() string {
	switch s {
	case Started:
		return "started"
	case Executing:
		return "executing"
	case Finished:
		return "finished"
	default:
		panic("impossible DKG state")
	}
}

type DKGRecord struct {
	BeaconID         string
	Epoch            uint32
	State            DKGState
	Time             time.Time
	SetupParams      *DKGSetupParams
	ExecutionParams  *DKGExecutionParams  // only present if State >= Executing
	CompletionParams *DKGCompletionParams // only present if State == Finished
}

type DKGSetupParams struct {
	ChainHash      []byte
	NodeCount      uint32
	Threshold      uint32
	Timeout        time.Time
	TransitionTime time.Time
	Leader         *LeaderInfo
}
type DKGSetupParamsTOML struct {
	ChainHash      []byte
	NodeCount      uint32
	Threshold      uint32
	Timeout        time.Time
	TransitionTime time.Time
	Leader         *LeaderInfo
}

type DKGExecutionParams struct {
	OldGroup string
	NewGroup string
}

type DKGCompletionParams struct {
	Group string
}

type LeaderInfo struct {
	IsSelf  bool
	Key     []byte
	Address string
	Tls     bool
}

type dkgStore struct {
	sync.RWMutex
	db *bolt.DB
}

const DBFileName = "dkg.db"
const DKGFolderName = "dkg"
const RWPerms = 0660

func NewDKGStore(baseFolder string, beaconID string, options *bolt.Options) (DKGStore, error) {
	dkgFolder := fs.CreateSecureFolder(path.Join(baseFolder, common.MultiBeaconFolder, beaconID, DKGFolderName))
	dbPath := path.Join(dkgFolder, DBFileName)
	db, err := bolt.Open(dbPath, RWPerms, options)
	if err != nil {
		return nil, err
	}

	return &dkgStore{db: db}, nil
}

func (s *dkgStore) Store(record *DKGRecord) error {
	s.Lock()
	defer s.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(record.BeaconID))
		if err != nil {
			return err
		}

		id, err := bucket.NextSequence()
		if err != nil {
			return err
		}

		idBytes := itob(id)
		buf := bytes.Buffer{}
		err = gob.NewEncoder(&buf).Encode(record)
		if err != nil {
			return err
		}
		return bucket.Put(idBytes, buf.Bytes())
	})
}

func itob(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func (s *dkgStore) Latest(beaconID string) (*DKGRecord, error) {
	s.RLock()
	defer s.RUnlock()

	var record DKGRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(beaconID))
		if bucket == nil {
			return nil
		}

		_, v := bucket.Cursor().Last()
		if v == nil {
			return nil
		}

		return gob.NewDecoder(bytes.NewReader(v)).Decode(&record)
	})

	// if the record has default fields, it did not exist in the DB
	if record.BeaconID == "" {
		return nil, nil
	}

	return &record, err
}

// History returns a list of DKG record for a given beaconID, or an empty array if the beaconID could not be found
// in the database
func (s *dkgStore) History(beaconID string) (*[]DKGRecord, error) {
	s.RLock()
	defer s.RUnlock()

	var records []DKGRecord

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(beaconID))
		if bucket == nil {
			return nil
		}

		err := bucket.ForEach(func(_, v []byte) error {
			record := DKGRecord{}
			err := gob.NewDecoder(bytes.NewReader(v)).Decode(&record)
			if err != nil {
				return err
			}
			records = append(records, record)
			return nil
		})
		if err != nil {
			return err
		}

		return nil
	})

	return &records, err
}
