package pg

import (
	"fmt"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/pg/dbtest"
	"github.com/drand/drand/chain/pg/docker"
)

var c *docker.Container

func TestMain(m *testing.M) {
	var err error
	c, err = dbtest.StartDB()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer dbtest.StopDB(c)

	m.Run()
}

func Test_OrderStorePG(t *testing.T) {
	beaconName := t.Name()

	log, db, teardown := dbtest.NewUnit(t, c, "testdb1")
	defer func() {
		if r := recover(); r != nil {
			t.Log(r)
			t.Error(string(debug.Stack()))
		}
		t.Cleanup(teardown)
	}()

	store, err := NewPGStore(log, db, beaconName)
	require.NoError(t, err)
	defer store.Close()

	b1 := &chain.Beacon{
		PreviousSig: []byte("a magnificent signature"),
		Round:       145,
		Signature:   []byte("one signature to"),
	}

	b2 := &chain.Beacon{
		PreviousSig: []byte("is not worth an invalid one"),
		Round:       146,
		Signature:   []byte("govern them all"),
	}

	// we store b2 and check if it is last
	require.NoError(t, store.Put(b2))
	eb2, err := store.Last()
	require.NoError(t, err)
	require.Equal(t, b2, eb2)
	eb2, err = store.Last()
	require.NoError(t, err)
	require.Equal(t, b2, eb2)

	// then we store b1
	require.NoError(t, store.Put(b1))

	// and request last again
	eb2, err = store.Last()
	require.NoError(t, err)
	require.Equal(t, b2, eb2)
}

func Test_StorePG(t *testing.T) {
	beaconName := t.Name()

	log, db, teardown := dbtest.NewUnit(t, c, "testdb2")
	defer func() {
		if r := recover(); r != nil {
			t.Log(r)
			t.Error(string(debug.Stack()))
		}
		t.Cleanup(teardown)
	}()

	store, err := NewPGStore(log, db, beaconName)
	require.NoError(t, err)
	defer store.Close()

	var sig1 = []byte{0x01, 0x02, 0x03}
	var sig2 = []byte{0x02, 0x03, 0x04}

	ln, err := store.Len()
	require.NoError(t, err)
	require.Equal(t, 0, ln)

	b1 := &chain.Beacon{
		PreviousSig: sig1,
		Round:       145,
		Signature:   sig2,
	}

	b2 := &chain.Beacon{
		PreviousSig: sig2,
		Round:       146,
		Signature:   sig1,
	}

	require.NoError(t, store.Put(b1))
	ln, err = store.Len()
	require.NoError(t, err)
	require.Equal(t, 1, ln)
	require.NoError(t, store.Put(b1))
	ln, _ = store.Len()
	require.Equal(t, 1, ln)
	require.NoError(t, store.Put(b2))
	ln, _ = store.Len()
	require.Equal(t, 2, ln)

	received, err := store.Last()
	require.NoError(t, err)
	require.Equal(t, b2, received)

	// =========================================================================

	store, err = NewPGStore(log, db, beaconName)
	require.NoError(t, err)
	defer store.Close()

	require.NoError(t, store.Put(b1))

	require.NoError(t, store.Put(b1))
	bb1, err := store.Get(b1.Round)
	require.NoError(t, err)
	require.Equal(t, b1, bb1)

	// =========================================================================

	store, err = NewPGStore(log, db, beaconName)
	require.NoError(t, err)
	defer store.Close()

	err = store.Put(b1)
	require.NoError(t, err)
	err = store.Put(b2)
	require.NoError(t, err)

	store.Cursor(func(c chain.Cursor) error {
		expecteds := []*chain.Beacon{b1, b2}
		i := 0
		b, err := c.First()
		for ; b != nil; b, err = c.Next() {
			require.NoError(t, err)
			require.True(t, expecteds[i].Equal(b))
			i++
		}

		unknown, err := c.Seek(10000)
		require.NotNil(t, err)
		require.Nil(t, unknown)
		return nil
	})

	err = store.Cursor(func(c chain.Cursor) error {
		lb2, err := c.Last()
		require.NoError(t, err)
		require.NotNil(t, lb2)
		require.Equal(t, b2, lb2)
		return nil
	})
	require.NoError(t, err)

	unknown, err := store.Get(10000)
	require.Nil(t, unknown)
	require.ErrorIs(t, err, ErrNoBeaconSaved)
}
