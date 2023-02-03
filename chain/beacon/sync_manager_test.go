package beacon

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/boltdb"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"
	dcontext "github.com/drand/drand/test/context"
	"github.com/stretchr/testify/require"
)

type testSyncStream struct {
	ctx        context.Context
	l          log.Logger
	counterMtx *sync.Mutex
	counter    int
	failOn     int
}

func (s *testSyncStream) Context() context.Context {
	return s.ctx
}

func (s *testSyncStream) GetCounter() int {
	s.counterMtx.Lock()
	defer s.counterMtx.Unlock()
	return s.counter
}

var errShouldFail = errors.New("should fail")

func (s *testSyncStream) Send(*drand.BeaconPacket) error {
	s.counterMtx.Lock()
	defer s.counterMtx.Unlock()
	s.counter++

	s.l.Debugw("sending message", "i", s.counter)

	if s.failOn == s.counter {
		s.l.Debugw("stream reached expected fail counter", "i", s.counter)
		return errShouldFail
	}
	if s.failOn != 0 &&
		s.failOn < s.counter {
		panic("shouldn't happen")
	}

	return nil
}

func createTestCBStore(t *testing.T) CallbackStore {
	t.Helper()
	dir := t.TempDir()
	ctx, _, _ := dcontext.PrevSignatureMattersOnContext(t, context.Background())
	l := test.Logger(t)
	bbstore, err := boltdb.NewBoltStore(ctx, l, dir, nil)
	require.NoError(t, err)
	cb := NewCallbackStore(bbstore)

	for i := uint64(0); i < 10; i++ {
		err := cb.Put(context.Background(), &chain.Beacon{
			PreviousSig: []byte("some sig"),
			Round:       i,
			Signature:   []byte("some sig"),
		})
		require.NoError(t, err)
	}

	return cb
}

//nolint:funlen
func TestSyncChain(t *testing.T) {
	t.Run("Running once", func(t *testing.T) {
		cb := createTestCBStore(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		l := test.Logger(t)
		stream := &testSyncStream{ctx: ctx, l: l.Named("1"), counterMtx: &sync.Mutex{}, counter: 0}
		errChan := make(chan error)

		go func() {
			errChan <- SyncChain(l, cb, &TestSyncRequest{round: 1}, stream)
		}()
		select {
		case err := <-errChan:
			require.NoError(t, err)
		case <-time.After(1 * time.Second):
			cancel()
		}
		err := <-errChan
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 9, stream.GetCounter())
	})

	t.Run("Running once with callback advancing", func(t *testing.T) {
		cb := createTestCBStore(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		l := test.Logger(t)
		stream := &testSyncStream{ctx: ctx, l: l.Named("1"), counterMtx: &sync.Mutex{}, counter: 0}
		errChan := make(chan error)

		go func() {
			errChan <- SyncChain(l, cb, &TestSyncRequest{round: 1}, stream)
		}()

		select {
		case err := <-errChan:
			require.NoError(t, err)
		case <-time.After(50 * time.Millisecond):
			for i := uint64(10); i < 15; i++ {
				err := cb.Put(context.Background(), &chain.Beacon{
					PreviousSig: []byte("some sig"),
					Round:       i,
					Signature:   []byte("some sig"),
				})
				require.NoError(t, err)
			}
			time.Sleep(50 * time.Millisecond)
			cancel()
		}

		err := <-errChan
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 14, stream.GetCounter())
	})

	t.Run("Running concurrently", func(t *testing.T) {
		cb := createTestCBStore(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		l := test.Logger(t)
		stream1 := &testSyncStream{ctx: ctx, l: l.Named("1"), counterMtx: &sync.Mutex{}, counter: 0}
		stream2 := &testSyncStream{ctx: ctx, l: l.Named("2"), counterMtx: &sync.Mutex{}, counter: 0}

		errChan1 := make(chan error)
		go func() {
			errChan1 <- SyncChain(l, cb, &TestSyncRequest{round: 1}, stream1)
		}()

		time.Sleep(50 * time.Millisecond)
		errChan2 := make(chan error)
		go func() {
			errChan2 <- SyncChain(l, cb, &TestSyncRequest{round: 1}, stream2)
		}()

		select {
		case err := <-errChan1:
			require.NoError(t, err)
		case err := <-errChan2:
			require.NoError(t, err)
		case <-time.After(50 * time.Millisecond):
			for i := uint64(10); i < 17; i++ {
				err := cb.Put(context.Background(), &chain.Beacon{
					PreviousSig: []byte("some sig"),
					Round:       i,
					Signature:   []byte("some sig"),
				})
				require.NoError(t, err)
			}
			time.Sleep(50 * time.Millisecond)
			cancel()
		}

		err := <-errChan1
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 16, stream1.GetCounter())
		require.Equal(t, 16, stream2.GetCounter())
	})

	t.Run("Running concurrently one stream fails", func(t *testing.T) {
		cb := createTestCBStore(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		l := test.Logger(t)
		stream1 := &testSyncStream{ctx: ctx, l: l.Named("1"), counterMtx: &sync.Mutex{}, counter: 0, failOn: 8}
		stream2 := &testSyncStream{ctx: ctx, l: l.Named("2"), counterMtx: &sync.Mutex{}, counter: 0}

		errChan1 := make(chan error)
		go func() {
			errChan1 <- SyncChain(l, cb, &TestSyncRequest{round: 1}, stream1)
		}()

		time.Sleep(50 * time.Millisecond)
		errChan2 := make(chan error)
		go func() {
			errChan2 <- SyncChain(l, cb, &TestSyncRequest{round: 1}, stream2)
		}()

		select {
		case err := <-errChan2:
			require.NoError(t, err)
		case <-time.After(50 * time.Millisecond):
			cancel()
		}

		err := <-errChan1
		require.ErrorIs(t, err, errShouldFail)
		require.Equal(t, 8, stream1.GetCounter())
		require.Equal(t, 9, stream2.GetCounter())
	})

	t.Run("Running concurrently one stream fails", func(t *testing.T) {
		cb := createTestCBStore(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		l := test.Logger(t)
		stream1 := &testSyncStream{ctx: ctx, l: l.Named("1"), counterMtx: &sync.Mutex{}, counter: 0, failOn: 13}
		stream2 := &testSyncStream{ctx: ctx, l: l.Named("2"), counterMtx: &sync.Mutex{}, counter: 0}

		errChan1 := make(chan error)
		go func() {
			errChan1 <- SyncChain(l, cb, &TestSyncRequest{round: 1}, stream1)
		}()

		time.Sleep(50 * time.Millisecond)

		errChan2 := make(chan error)
		go func() {
			errChan2 <- SyncChain(l, cb, &TestSyncRequest{round: 1}, stream2)
		}()

		select {
		case err := <-errChan2:
			require.NoError(t, err)
		case <-time.After(50 * time.Millisecond):
			for i := uint64(10); i < 17; i++ {
				err := cb.Put(context.Background(), &chain.Beacon{
					PreviousSig: []byte("some sig"),
					Round:       i,
					Signature:   []byte("some sig"),
				})
				require.NoError(t, err)
				// TODO: make sure the callbacks are not able to "keep running" after being removed
				time.Sleep(5 * time.Millisecond)
			}
			time.Sleep(50 * time.Millisecond)
			cancel()
		}
		err := <-errChan1
		require.ErrorIs(t, err, errShouldFail)
		require.Equal(t, 13, stream1.GetCounter())
		require.Equal(t, 16, stream2.GetCounter())
	})
}
