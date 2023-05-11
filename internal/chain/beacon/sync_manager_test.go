package beacon

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/peer"

	"github.com/drand/drand/common"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/chain/boltdb"
	dcontext "github.com/drand/drand/internal/test/context"
	"github.com/drand/drand/internal/test/testlogger"
	"github.com/drand/drand/protobuf/drand"
)

type testSyncStream struct {
	ctx        context.Context
	t          *testing.T
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

	require.False(s.t, s.failOn != 0 && s.failOn < s.counter, "shouldn't happen")

	return nil
}

func createTestCBStore(t *testing.T) CallbackStore {
	t.Helper()
	dir := t.TempDir()
	ctx, _, _ := dcontext.PrevSignatureMattersOnContext(t, context.Background())
	l := testlogger.New(t)
	bbstore, err := boltdb.NewBoltStore(ctx, l, dir, nil)
	require.NoError(t, err)
	cb := NewCallbackStore(l, bbstore)

	for i := uint64(0); i < 10; i++ {
		err := cb.Put(context.Background(), &common.Beacon{

			Round: i,
		})
		require.NoError(t, err)
	}

	return cb
}

func peerCtx(ctx context.Context, t *testing.T, addr string) context.Context {
	_, p1Addr, err := net.ParseCIDR(addr)
	require.NoError(t, err)

	p := peer.Peer{Addr: p1Addr}
	return peer.NewContext(ctx, &p)
}

func TestSyncChainSinglePeer(t *testing.T) {
	addr1 := "192.168.0.11/32"

	doTest(t, addr1, addr1)
}

func TestSyncChainTwoPeers(t *testing.T) {
	addr1 := "192.168.0.11/32"
	addr2 := "192.168.0.12/32"

	doTest(t, addr1, addr2)
}

//nolint:funlen,maintidx
func doTest(t *testing.T, addr1, addr2 string) {
	t.Run("Running once", func(t *testing.T) {
		cb := createTestCBStore(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		l := testlogger.New(t)
		stream := &testSyncStream{
			ctx:        peerCtx(ctx, t, addr1),
			t:          t,
			l:          l.Named("1"),
			counterMtx: &sync.Mutex{},
		}
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

		l := testlogger.New(t)
		stream := &testSyncStream{
			ctx:        peerCtx(ctx, t, addr1),
			t:          t,
			l:          l.Named("1"),
			counterMtx: &sync.Mutex{},
		}
		errChan := make(chan error)

		go func() {
			errChan <- SyncChain(l, cb, &TestSyncRequest{round: 1}, stream)
		}()

		select {
		case err := <-errChan:
			require.NoError(t, err)
		case <-time.After(50 * time.Millisecond):
			for i := uint64(10); i < 15; i++ {
				err := cb.Put(context.Background(), &common.Beacon{
					Round: i,
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

		l := testlogger.New(t)
		stream1 := &testSyncStream{
			ctx:        peerCtx(ctx, t, addr1),
			t:          t,
			l:          l.Named("1"),
			counterMtx: &sync.Mutex{},
		}
		stream2 := &testSyncStream{
			ctx:        peerCtx(ctx, t, addr2),
			t:          t,
			l:          l.Named("2"),
			counterMtx: &sync.Mutex{},
		}

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
				err := cb.Put(context.Background(), &common.Beacon{
					Round: i,
				})
				require.NoError(t, err)
			}
			time.Sleep(50 * time.Millisecond)
			cancel()
		}

		err := <-errChan1
		// Here we expect all peers to complete the full sync.
		// However, when we test a single peer exclusively, the following events will happen:
		// - the first stream will start consuming everything as soon as possible, meaning all values
		// - the second stream joins, replacing the first stream, consuming everything as soon as possible
		// - then, we start producing new value for beacons
		// - because the first stream was replaced by the second one, we won't receive any new values.
		//
		// This is the correct/expected behavior.
		if addr1 == addr2 {
			require.ErrorIs(t, err, ErrCallbackReplaced)
		} else {
			require.ErrorIs(t, err, context.Canceled)
		}
		expected := 16
		if addr1 == addr2 {
			expected = 9
		}
		require.Equal(t, expected, stream1.GetCounter())

		err = <-errChan2
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 16, stream2.GetCounter())
	})

	t.Run("Running concurrently one stream fails with no new values produced", func(t *testing.T) {
		cb := createTestCBStore(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		l := testlogger.New(t)
		stream1 := &testSyncStream{
			ctx:        peerCtx(ctx, t, addr1),
			t:          t,
			l:          l.Named("1"),
			counterMtx: &sync.Mutex{},
			failOn:     8,
		}
		stream2 := &testSyncStream{
			ctx:        peerCtx(ctx, t, addr2),
			t:          t,
			l:          l.Named("2"),
			counterMtx: &sync.Mutex{},
		}

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

		err = <-errChan2
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 9, stream2.GetCounter())
	})

	t.Run("Running concurrently one stream fails", func(t *testing.T) {
		cb := createTestCBStore(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		l := testlogger.New(t)
		stream1 := &testSyncStream{
			ctx:        peerCtx(ctx, t, addr1),
			t:          t,
			l:          l.Named("1"),
			counterMtx: &sync.Mutex{},
			failOn:     13,
		}
		stream2 := &testSyncStream{
			ctx:        peerCtx(ctx, t, addr2),
			t:          t,
			l:          l.Named("2"),
			counterMtx: &sync.Mutex{},
		}

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
				err := cb.Put(context.Background(), &common.Beacon{
					Round: i,
				})
				require.NoError(t, err)
				// TODO: make sure the callbacks are not able to "keep running" after being removed
				time.Sleep(5 * time.Millisecond)
			}
			time.Sleep(50 * time.Millisecond)
			cancel()
		}

		err := <-errChan1
		// Here we expect all peers to complete the full sync.
		// However, when we test a single peer exclusively, the following events will happen:
		// - the first stream will start consuming everything as soon as possible, meaning all values
		// - the second stream joins, replacing the first stream, consuming everything as soon as possible
		// - then, we start producing new value for beacons
		// - because the first stream was replaced by the second one, we won't receive any new values.
		//
		// This is the correct/expected behavior.
		if addr1 == addr2 {
			require.ErrorIs(t, err, ErrCallbackReplaced)
		} else {
			require.ErrorIs(t, err, errShouldFail)
		}

		expected := 13
		if addr1 == addr2 {
			expected = 9
		}
		require.Equal(t, expected, stream1.GetCounter())

		err = <-errChan2
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 16, stream2.GetCounter())
	})
}
