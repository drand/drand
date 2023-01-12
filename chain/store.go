package chain

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
)

// store contains all the definitions and implementation of the logic that
// stores and loads beacon signatures. At the moment of writing, it consists of
// a boltdb key/value database store.

// Store is an interface to store Beacons packets where they can also be
// retrieved to be delivered to end clients.
type Store interface {
	Len(context.Context) (int, error)
	Put(context.Context, *Beacon) error
	Last(context.Context) (*Beacon, error)
	Get(ctx context.Context, round uint64) (*Beacon, error)
	Cursor(context.Context, func(context.Context, Cursor) error) error
	Close(context.Context) error
	Del(ctx context.Context, round uint64) error
	SaveTo(ctx context.Context, w io.Writer) error
}

// Cursor iterates over items in sorted key order. This starts from the
// first key/value pair and updates the k/v variables to the
// next key/value on each iteration.
//
// The loop finishes at the end of the cursor when a nil key is returned.
//
//	for k, v := c.First(); k != nil; k, v = c.Next() {
//	    fmt.Printf("A %s is %s.\n", k, v)
//	}
type Cursor interface {
	First(context.Context) (*Beacon, error)
	Next(context.Context) (*Beacon, error)
	Seek(ctx context.Context, round uint64) (*Beacon, error)
	Last(context.Context) (*Beacon, error)
}

// StorageType defines the supported storage engines
type StorageType string

// Storage engine types
const (
	// BoltDB uses the BoltDB engine for storing data
	BoltDB StorageType = "bolt"

	// PostgreSQL uses the PostgreSQL database for storing data
	PostgreSQL StorageType = "postgres"

	// MemDB uses the in-memory database to store data
	MemDB StorageType = "memdb"
)

// Metrics values for reporting storage type used
const (
	boltDBMetrics = iota + 1
	postgreSQLMetrics
	memDBMetrics
)

func MetricsStorageType(st StorageType) int {
	switch st {
	case BoltDB:
		return boltDBMetrics
	case PostgreSQL:
		return postgreSQLMetrics
	case MemDB:
		return memDBMetrics
	default:
		err := fmt.Errorf("unknown storage type %q for metrics reporting", st)
		// Please add the storage type to the Metrics values list above
		panic(err)
	}
}

// RoundToBytes serializes a round number to bytes (8 bytes fixed length big-endian).
func RoundToBytes(r uint64) []byte {
	var buff bytes.Buffer
	_ = binary.Write(&buff, binary.BigEndian, r)
	return buff.Bytes()
}

// GenesisBeacon returns the first beacon inserted in the chain
func GenesisBeacon(c *Info) *Beacon {
	return &Beacon{
		Signature: c.GenesisSeed,
		Round:     0,
	}
}
