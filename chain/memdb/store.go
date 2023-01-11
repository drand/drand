package memdb

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/errors"
)

// Store represents access to the in-memory storage for beacon management.
type Store struct {
	storeMtx   *sync.RWMutex
	store      []*chain.Beacon
	bufferSize int
}

// NewStore returns a new store that provides the CRUD based API needed for
// supporting drand serialization.
func NewStore(bufferSize int) *Store {
	if bufferSize < 1 {
		err := fmt.Errorf("in-memory buffer size cannot be smaller than 1, currently %d, recommended at least 2000", bufferSize)
		panic(err)
	}
	return &Store{
		storeMtx:   &sync.RWMutex{},
		store:      make([]*chain.Beacon, 0, bufferSize),
		bufferSize: bufferSize,
	}
}

func (s *Store) Len(_ context.Context) (int, error) {
	s.storeMtx.RLock()
	defer s.storeMtx.RUnlock()

	return len(s.store), nil
}

func (s *Store) Put(_ context.Context, beacon *chain.Beacon) error {
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

func (s *Store) Last(_ context.Context) (*chain.Beacon, error) {
	s.storeMtx.RLock()
	defer s.storeMtx.RUnlock()

	if len(s.store) == 0 {
		return nil, errors.ErrNoBeaconStored
	}

	result := s.store[len(s.store)-1]
	return result, nil
}

func (s *Store) Get(_ context.Context, round uint64) (*chain.Beacon, error) {
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
	cursor := &memDBCursor{
		s: s,
	}
	return f(ctx, cursor)
}

// Close is a noop
func (s *Store) Close(_ context.Context) error {
	return nil
}

func (s *Store) Del(_ context.Context, round uint64) error {
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

func (s *Store) SaveTo(ctx context.Context, w io.Writer) error {
	// TODO implement me
	panic("implement me")
}

type memDBCursor struct {
	s   *Store
	pos int
}

func (m *memDBCursor) First(_ context.Context) (*chain.Beacon, error) {
	m.s.storeMtx.RLock()
	defer m.s.storeMtx.RUnlock()

	if len(m.s.store) == 0 {
		return nil, errors.ErrNoBeaconStored
	}

	m.pos = 0
	result := m.s.store[m.pos]
	return result, nil
}

func (m *memDBCursor) Next(_ context.Context) (*chain.Beacon, error) {
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

func (m *memDBCursor) Seek(_ context.Context, round uint64) (*chain.Beacon, error) {
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

func (m *memDBCursor) Last(_ context.Context) (*chain.Beacon, error) {
	m.s.storeMtx.RLock()
	defer m.s.storeMtx.RUnlock()

	if len(m.s.store) == 0 {
		return nil, errors.ErrNoBeaconStored
	}

	m.pos = len(m.s.store) - 1
	result := m.s.store[m.pos]
	return result, nil
}
