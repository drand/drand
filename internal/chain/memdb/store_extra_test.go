//go:build memdb

package memdb_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/internal/chain"
	chainerrors "github.com/drand/drand/v2/internal/chain/errors"
	"github.com/drand/drand/v2/internal/chain/memdb"
)

func mkBeacon(round uint64) *common.Beacon {
	return &common.Beacon{
		PreviousSig: []byte{byte(round - 1)},
		Round:       round,
		Signature:   []byte{byte(round)},
	}
}

// errWriter always fails on Write, to exercise SaveTo's error path.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, io.ErrShortWrite }

func TestNewStorePanicsOnSmallBuffer(t *testing.T) {
	require.Panics(t, func() { memdb.NewStore(0) })
	require.Panics(t, func() { memdb.NewStore(9) })
	require.NotPanics(t, func() { memdb.NewStore(10) })
}

func TestStoreDel(t *testing.T) {
	ctx := context.Background()
	s := memdb.NewStore(10)

	b1, b2, b3 := mkBeacon(1), mkBeacon(2), mkBeacon(3)
	require.NoError(t, s.Put(ctx, b1))
	require.NoError(t, s.Put(ctx, b2))
	require.NoError(t, s.Put(ctx, b3))

	// Deleting a round that does not exist is a no-op (no error).
	require.NoError(t, s.Del(ctx, 999))
	l, err := s.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, l)

	// Delete a middle element and verify ordering/contents are preserved.
	require.NoError(t, s.Del(ctx, 2))
	l, err = s.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, l)

	_, err = s.Get(ctx, 2)
	require.ErrorIs(t, err, chainerrors.ErrNoBeaconStored)

	got1, err := s.Get(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, b1, got1)
	got3, err := s.Get(ctx, 3)
	require.NoError(t, err)
	require.Equal(t, b3, got3)

	// Delete the last remaining elements down to empty.
	require.NoError(t, s.Del(ctx, 1))
	require.NoError(t, s.Del(ctx, 3))
	l, err = s.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, l)

	_, err = s.Last(ctx)
	require.ErrorIs(t, err, chainerrors.ErrNoBeaconStored)
}

func TestStoreSaveToContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := memdb.NewStore(10)
	require.NoError(t, s.Put(ctx, mkBeacon(1)))

	cancel()
	err := s.SaveTo(ctx, errWriter{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestStoreSaveToWriteError(t *testing.T) {
	ctx := context.Background()
	s := memdb.NewStore(10)
	require.NoError(t, s.Put(ctx, mkBeacon(1)))

	err := s.SaveTo(ctx, errWriter{})
	require.Error(t, err)
	require.ErrorIs(t, err, io.ErrShortWrite)
}

// TestStorePutBufferTrim verifies the ring-buffer behaviour: when more than
// bufferSize beacons are inserted, only the highest-round ones are retained.
func TestStorePutBufferTrim(t *testing.T) {
	ctx := context.Background()
	const buf = 10
	s := memdb.NewStore(buf)

	for r := uint64(1); r <= 25; r++ {
		require.NoError(t, s.Put(ctx, mkBeacon(r)))
	}

	l, err := s.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, buf, l)

	// The oldest rounds must have been trimmed away.
	_, err = s.Get(ctx, 1)
	require.ErrorIs(t, err, chainerrors.ErrNoBeaconStored)

	// The newest round must be present and be Last.
	last, err := s.Last(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(25), last.Round)

	got16, err := s.Get(ctx, 16)
	require.NoError(t, err)
	require.Equal(t, uint64(16), got16.Round)
}

// TestStorePutOutOfOrder verifies a beacon inserted with a smaller round than
// the current last triggers a re-sort so the store stays ordered.
func TestStorePutOutOfOrder(t *testing.T) {
	ctx := context.Background()
	s := memdb.NewStore(10)

	require.NoError(t, s.Put(ctx, mkBeacon(5)))
	require.NoError(t, s.Put(ctx, mkBeacon(3)))
	require.NoError(t, s.Put(ctx, mkBeacon(4)))

	last, err := s.Last(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(5), last.Round)

	// The store must be sorted by round despite the out-of-order insertion.
	var rounds []uint64
	err = s.Cursor(ctx, func(ctx context.Context, c chain.Cursor) error {
		b, err := c.First(ctx)
		for ; b != nil; b, err = c.Next(ctx) {
			require.NoError(t, err)
			rounds = append(rounds, b.Round)
		}
		if !errors.Is(err, chainerrors.ErrNoBeaconStored) {
			require.NoError(t, err)
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []uint64{3, 4, 5}, rounds)
}

func TestStoreCursorEmpty(t *testing.T) {
	ctx := context.Background()
	s := memdb.NewStore(10)

	err := s.Cursor(ctx, func(ctx context.Context, c chain.Cursor) error {
		_, err := c.First(ctx)
		require.ErrorIs(t, err, chainerrors.ErrNoBeaconStored)

		_, err = c.Next(ctx)
		require.ErrorIs(t, err, chainerrors.ErrNoBeaconStored)

		_, err = c.Last(ctx)
		require.ErrorIs(t, err, chainerrors.ErrNoBeaconStored)

		_, err = c.Seek(ctx, 1)
		require.ErrorIs(t, err, chainerrors.ErrNoBeaconStored)
		return nil
	})
	require.NoError(t, err)
}

func TestStoreCursorErrorPropagates(t *testing.T) {
	ctx := context.Background()
	s := memdb.NewStore(10)
	require.NoError(t, s.Put(ctx, mkBeacon(1)))

	sentinel := errors.New("boom")
	err := s.Cursor(ctx, func(ctx context.Context, c chain.Cursor) error {
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)
}
