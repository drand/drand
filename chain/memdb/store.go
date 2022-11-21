package memdb

import (
	"context"
	"io"
	"sort"
	"sync"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/errors"
)

// Store ...
type Store struct {
	storeMtx *sync.RWMutex
	store    []chain.Beacon
	maxSize  int
}

// NewStore ...
func NewStore() *Store {
	const maxSize = 10

	return &Store{
		storeMtx: &sync.RWMutex{},
		store:    []chain.Beacon{},
		maxSize:  maxSize,
	}
}

func (m *Store) Len(_ context.Context) (int, error) {
	m.storeMtx.RLock()
	defer m.storeMtx.RUnlock()

	return len(m.store), nil
}

func (m *Store) Put(_ context.Context, beacon *chain.Beacon) error {
	m.storeMtx.Lock()
	defer m.storeMtx.Unlock()
	defer func() {
		if len(m.store) > m.maxSize {
			m.store = m.store[len(m.store)-m.maxSize:]
		}
	}()

	found := false
	defer func() {
		sort.Slice(m.store, func(i, j int) bool {
			return m.store[i].Round < m.store[j].Round
		})
	}()

	for _, sb := range m.store {
		if sb.Round == beacon.Round {
			found = true
			break
		}
	}

	if found {
		return nil
	}

	m.store = append(m.store, *beacon)
	return nil
}

func (m *Store) Last(_ context.Context) (*chain.Beacon, error) {
	m.storeMtx.RLock()
	defer m.storeMtx.RUnlock()

	if len(m.store) == 0 {
		return nil, errors.ErrNoBeaconStored
	}

	result := m.store[len(m.store)-1]
	return &result, nil
}

func (m *Store) Get(_ context.Context, round uint64) (*chain.Beacon, error) {
	m.storeMtx.RLock()
	defer m.storeMtx.RUnlock()

	for _, beacon := range m.store {
		if beacon.Round == round {
			return &beacon, nil
		}

	}

	return nil, errors.ErrNoBeaconStored
}

func (m *Store) Cursor(ctx context.Context, f func(context.Context, chain.Cursor) error) error {
	cursor := &memDBCursor{
		s: m,
	}
	return f(ctx, cursor)
}

// Close is a noop
func (m *Store) Close(_ context.Context) error {
	return nil
}

func (m *Store) Del(_ context.Context, round uint64) error {
	m.storeMtx.Lock()
	defer m.storeMtx.Unlock()

	foundIdx := -1
	for idx, beacon := range m.store {
		if beacon.Round == round {
			foundIdx = idx
			break
		}
	}

	if foundIdx == -1 {
		return nil
	}

	m.store = append(m.store[:foundIdx], m.store[foundIdx+1:]...)

	return nil
}

func (m *Store) SaveTo(ctx context.Context, w io.Writer) error {
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
	return &result, nil
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
	return &result, nil
}

func (m *memDBCursor) Seek(_ context.Context, round uint64) (*chain.Beacon, error) {
	m.s.storeMtx.RLock()
	defer m.s.storeMtx.RUnlock()

	for idx, beacon := range m.s.store {
		if beacon.Round != round {
			continue
		}

		m.pos = idx
		return &beacon, nil
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
	return &result, nil
}
