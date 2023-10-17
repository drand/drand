package memdb

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"

	"github.com/drand/drand/common/tracer"

	"github.com/drand/drand/common"
	"github.com/drand/drand/internal/chain"
	"github.com/drand/drand/internal/chain/errors"
)

// Store represents access to the in-memory storage for beacon management.
type Store struct {
	storeMtx   *sync.RWMutex
	store      []*common.Beacon
	bufferSize int
}

// NewStore returns a new store that provides the CRUD based API needed for
// supporting drand serialization.
func NewStore(bufferSize int) *Store {
	//nolint:gomnd // We want to have a guard here. And it's number 10. It's higher than 1 or 2 to allow for chained mode
	if bufferSize < 10 {
		err := fmt.Errorf("in-memory buffer size cannot be smaller than 10, currently %d, recommended at least 2000", bufferSize)
		panic(err)
	}
	return &Store{
		storeMtx:   &sync.RWMutex{},
		store:      make([]*common.Beacon, 0, bufferSize),
		bufferSize: bufferSize,
	}
}

func (s *Store) Len(ctx context.Context) (int, error) {
	_, span := tracer.NewSpan(ctx, "memDB.Len")
	defer span.End()

	s.storeMtx.RLock()
	defer s.storeMtx.RUnlock()

	return len(s.store), nil
}

func (s *Store) Put(ctx context.Context, beacon *common.Beacon) error {
	_, span := tracer.NewSpan(ctx, "memDB.Put")
	defer span.End()

	s.storeMtx.Lock()
	defer s.storeMtx.Unlock()
	defer func() {
		if len(s.store) > s.bufferSize {
			s.store = s.store[len(s.store)-s.bufferSize:]
		}
	}()

	for _, sb := range s.store {
		if sb.Round == beacon.Round {
			return nil
		}
	}

	shouldSort := false
	if len(s.store) > 0 &&
		beacon.Round < s.store[len(s.store)-1].Round {
		shouldSort = true
	}
	s.store = append(s.store, beacon)
	if shouldSort {
		sort.Slice(s.store, func(i, j int) bool {
			return s.store[i].Round < s.store[j].Round
		})
	}

	return nil
}

func (s *Store) Last(ctx context.Context) (*common.Beacon, error) {
	_, span := tracer.NewSpan(ctx, "memDB.Last")
	defer span.End()

	s.storeMtx.RLock()
	defer s.storeMtx.RUnlock()

	if len(s.store) == 0 {
		return nil, errors.ErrNoBeaconStored
	}

	result := s.store[len(s.store)-1]
	return result, nil
}

func (s *Store) Get(ctx context.Context, round uint64) (*common.Beacon, error) {
	_, span := tracer.NewSpan(ctx, "memDB.Get")
	defer span.End()

	s.storeMtx.RLock()
	defer s.storeMtx.RUnlock()

	for _, beacon := range s.store {
		if beacon.Round == round {
			return beacon, nil
		}
	}

	return nil, errors.ErrNoBeaconStored
}

func (s *Store) Cursor(ctx context.Context, f func(context.Context, chain.Cursor) error) error {
	ctx, span := tracer.NewSpan(ctx, "memDB.Cursor")
	defer span.End()

	cursor := &memDBCursor{
		s: s,
	}
	return f(ctx, cursor)
}

// Close is a noop
func (s *Store) Close() error {
	return nil
}

func (s *Store) Del(ctx context.Context, round uint64) error {
	_, span := tracer.NewSpan(ctx, "memDB.Del")
	defer span.End()

	s.storeMtx.Lock()
	defer s.storeMtx.Unlock()

	foundIdx := -1
	for idx, beacon := range s.store {
		if beacon.Round == round {
			foundIdx = idx
			break
		}
	}

	if foundIdx == -1 {
		return nil
	}

	s.store = append(s.store[:foundIdx], s.store[foundIdx+1:]...)

	return nil
}

func (s *Store) SaveTo(ctx context.Context, _ io.Writer) error {
	_, span := tracer.NewSpan(ctx, "memDB.SaveTo")
	defer span.End()

	// TODO implement me
	return fmt.Errorf("saveTo not implemented for MemDB Store")
}

type memDBCursor struct {
	s   *Store
	pos int
}

func (m *memDBCursor) First(ctx context.Context) (*common.Beacon, error) {
	_, span := tracer.NewSpan(ctx, "memDB.Cursor.First")
	defer span.End()

	m.s.storeMtx.RLock()
	defer m.s.storeMtx.RUnlock()

	if len(m.s.store) == 0 {
		return nil, errors.ErrNoBeaconStored
	}

	m.pos = 0
	result := m.s.store[m.pos]
	return result, nil
}

func (m *memDBCursor) Next(ctx context.Context) (*common.Beacon, error) {
	_, span := tracer.NewSpan(ctx, "memDB.Cursor.Next")
	defer span.End()

	m.s.storeMtx.RLock()
	defer m.s.storeMtx.RUnlock()

	if len(m.s.store) == 0 {
		return nil, errors.ErrNoBeaconStored
	}

	m.pos++
	if m.pos >= len(m.s.store) {
		return nil, errors.ErrNoBeaconStored
	}

	result := m.s.store[m.pos]
	return result, nil
}

func (m *memDBCursor) Seek(ctx context.Context, round uint64) (*common.Beacon, error) {
	_, span := tracer.NewSpan(ctx, "memDB.Cursor.Seek")
	defer span.End()

	m.s.storeMtx.RLock()
	defer m.s.storeMtx.RUnlock()

	for idx, beacon := range m.s.store {
		if beacon.Round != round {
			continue
		}

		m.pos = idx
		return beacon, nil
	}

	return nil, errors.ErrNoBeaconStored
}

func (m *memDBCursor) Last(ctx context.Context) (*common.Beacon, error) {
	_, span := tracer.NewSpan(ctx, "memDB.Cursor.Last")
	defer span.End()

	m.s.storeMtx.RLock()
	defer m.s.storeMtx.RUnlock()

	if len(m.s.store) == 0 {
		return nil, errors.ErrNoBeaconStored
	}

	m.pos = len(m.s.store) - 1
	result := m.s.store[m.pos]
	return result, nil
}
