package beacon

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/protobuf/drand"
)

// partialCache is a cache that stores (or not) all the partials the node
// receives.
// The partialCache contains some logic to prevent a DDOS attack on the partial
// signatures cache. Namely, it makes sure that there is a limited number of
// partial signatures from the same index stored at any given time.
type partialCache struct {
	rounds map[string]*roundCache
	rcvd   map[int][]string
	l      log.Logger
	scheme *crypto.Scheme
}

func newPartialCache(l log.Logger, s *crypto.Scheme) *partialCache {
	return &partialCache{
		rounds: make(map[string]*roundCache),
		rcvd:   make(map[int][]string),
		l:      l,
		scheme: s,
	}
}

func roundID(round uint64, previous []byte) string {
	var buff bytes.Buffer
	_ = binary.Write(&buff, binary.BigEndian, round)
	_, _ = buff.Write(previous)
	return buff.String()
}

// Append adds a partial signature to the cache.
func (c *partialCache) Append(p *drand.PartialBeaconPacket) error {
	id := roundID(p.GetRound(), p.GetPreviousSignature())
	idx, err := c.scheme.ThresholdScheme.IndexOf(p.GetPartialSig())
	if err != nil {
		c.l.Errorw("partialCache could not get index of threshold scheme", "err", err)
		return err
	}
	round, err := c.getCache(id, p)
	if round == nil || err != nil {
		return fmt.Errorf("could not get round from cache: %w", err)
	}
	if round.append(p) {
		// we increment the counter of that node index
		c.rcvd[idx] = append(c.rcvd[idx], id)
	}
	return nil
}

// FlushRounds deletes all rounds cache that are inferior or equal to `round`.
func (c *partialCache) FlushRounds(round uint64) {
	for id, cache := range c.rounds {
		if cache.round > round {
			continue
		}

		// delete the cache entry
		delete(c.rounds, id)
		// delete the counter of each nodes that participated in that round
		for idx := range cache.sigs {
			var idSlice = c.rcvd[idx][:0]
			for _, idd := range c.rcvd[idx] {
				if idd == id {
					continue
				}
				idSlice = append(idSlice, idd)
			}
			if len(idSlice) > 0 {
				c.rcvd[idx] = idSlice
			} else {
				delete(c.rcvd, idx)
			}
		}
	}
}

func (c *partialCache) GetRoundCache(round uint64, previous []byte) *roundCache {
	id := roundID(round, previous)
	return c.rounds[id]
}

// newRoundCache creates a new round cache given p. If the signer of the partial
// already has more than `
func (c *partialCache) getCache(id string, p *drand.PartialBeaconPacket) (*roundCache, error) {
	if round, ok := c.rounds[id]; ok {
		return round, nil
	}

	idx, err := c.scheme.ThresholdScheme.IndexOf(p.GetPartialSig())
	if err != nil {
		c.l.Errorw("partial cache miss", "beacon_id", id, "index", idx, "not_present_for", p.GetRound())
		return nil, err
	}
	if len(c.rcvd[idx]) >= MaxPartialsPerNode {
		// this node has submitted too many partials - we take the last one off
		toEvict := c.rcvd[idx][0]
		round, ok := c.rounds[toEvict]
		if !ok {
			c.l.Errorw("evicted round missing from cache", "beacon_id", id, "toEvict", toEvict, "node", idx, "not_present_for", p.GetRound())
			return nil, fmt.Errorf("evicted round missing from cache")
		}
		round.flushIndex(idx)
		c.rcvd[idx] = append(c.rcvd[idx][1:], id)
		// if the round is now empty, delete it
		if round.Len() == 0 {
			delete(c.rounds, toEvict)
		}
	}
	round := newRoundCache(id, p, c.scheme)
	c.rounds[id] = round
	return round, nil
}

type roundCache struct {
	round  uint64
	prev   []byte
	id     string
	sigs   map[int][]byte
	scheme *crypto.Scheme
}

func newRoundCache(id string, p *drand.PartialBeaconPacket, s *crypto.Scheme) *roundCache {
	return &roundCache{
		round:  p.GetRound(),
		prev:   p.GetPreviousSignature(),
		id:     id,
		sigs:   make(map[int][]byte),
		scheme: s,
	}
}

func (r *roundCache) GetRound() uint64 {
	return r.round
}

func (r *roundCache) GetPreviousSignature() []byte {
	return r.prev
}

// append stores the partial and returns true if the partial is not stored . It
// returns false if the cache is already caching this partial signature.
func (r *roundCache) append(p *drand.PartialBeaconPacket) bool {
	idx, err := r.scheme.ThresholdScheme.IndexOf(p.GetPartialSig())
	if err != nil {
		return false
	}
	if _, seen := r.sigs[idx]; seen {
		return false
	}
	r.sigs[idx] = p.GetPartialSig()
	return true
}

// Len shows how many items are in the cache
func (r *roundCache) Len() int {
	return len(r.sigs)
}

// Partials provides all cached partial signatures
func (r *roundCache) Partials() [][]byte {
	partials := make([][]byte, 0, len(r.sigs))
	for _, sig := range r.sigs {
		partials = append(partials, sig)
	}
	return partials
}

func (r *roundCache) flushIndex(idx int) {
	delete(r.sigs, idx)
}
