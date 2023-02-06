package beacon

import (
	"context"
	"errors"
	"github.com/drand/drand/chain"
	chainerrors "github.com/drand/drand/chain/errors"
	"io"
)

type WatchStore struct {
	store  chain.Store
	stream chan chain.Beacon
}

func NewWatchStore(store chain.Store) WatchStore {
	return WatchStore{
		store:  store,
		stream: make(chan chain.Beacon),
	}
}

func (w *WatchStore) Stream() <-chan chain.Beacon {
	return w.stream
}

func (w *WatchStore) Len(ctx context.Context) (int, error) {
	return w.store.Len(ctx)
}

func (w *WatchStore) AllStream(ctx context.Context) (<-chan chain.Beacon, error) {
	innerCtx, cancel := context.WithCancel(ctx)
	out := make(chan chain.Beacon)
	interim := make(chan chain.Beacon)

	go func() {
		for {
			select {
			case <-innerCtx.Done():
				break
			case newBeacon := <-w.Stream():
				interim <- newBeacon
			}
		}
	}()

	err := w.Cursor(innerCtx, func(ctx context.Context, cursor chain.Cursor) error {
		for {
			next, err := cursor.Next(ctx)
			if err != nil {
				if errors.Is(err, chainerrors.ErrNoBeaconStored) {
					break
				}
				return err
			}
			out <- *next
		}
		return nil
	})

	cancel()

	if err != nil {
		return out, err
	}

	for beacon := range interim {
		out <- beacon
	}

	return out, nil
}

func (w *WatchStore) Put(ctx context.Context, beacon *chain.Beacon) error {
	err := w.store.Put(ctx, beacon)
	if err != nil {
		return err
	}
	w.stream <- *beacon
	return nil
}

func (w *WatchStore) Last(ctx context.Context) (*chain.Beacon, error) {
	return w.store.Last(ctx)
}

func (w *WatchStore) Get(ctx context.Context, round uint64) (*chain.Beacon, error) {
	return w.store.Get(ctx, round)
}

func (w *WatchStore) Cursor(ctx context.Context, f func(context.Context, chain.Cursor) error) error {
	return w.store.Cursor(ctx, f)
}

func (w *WatchStore) Close(ctx context.Context) error {
	close(w.stream)
	return w.store.Close(ctx)
}

func (w *WatchStore) Del(ctx context.Context, round uint64) error {
	return w.store.Del(ctx, round)
}

func (w *WatchStore) SaveTo(ctx context.Context, writer io.Writer) error {
	return w.store.SaveTo(ctx, writer)
}
