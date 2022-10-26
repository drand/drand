package chain

import (
	"bytes"
	"encoding/binary"
	"io"
)

// store contains all the definitions and implementation of the logic that
// stores and loads beacon signatures. At the moment of writing, it consists of
// a boltdb key/value database store.

// Store is an interface to store Beacons packets where they can also be
// retrieved to be delivered to end clients.
type Store interface {
	Len() (int, error)
	Put(*Beacon) error
	Last() (*Beacon, error)
	Get(round uint64) (*Beacon, error)
	Cursor(func(Cursor) error) error
	Close() error
	Del(round uint64) error
	SaveTo(w io.Writer) error
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
	First() (*Beacon, error)
	Next() (*Beacon, error)
	Seek(round uint64) (*Beacon, error)
	Last() (*Beacon, error)
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
