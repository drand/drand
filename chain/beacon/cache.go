package beacon

import (
	"bytes"
	"encoding/binary"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
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
}

func newPartialCache(l log.Logger) *partialCache {
	return &partialCache{
		rounds: make(map[string]*roundCache),
		rcvd:   make(map[int][]string),
		l:      l,
	}
}

func roundID(round uint64, previous []byte) string {
	var buff bytes.Buffer
	_ = binary.Write(&buff, binary.BigEndian, round)
	_, _ = buff.Write(previous)
	return buff.String()
}

// Append adds a partial signature to the cache.
func (c *partialCache) Append(p *drand.PartialBeaconPacket) {
	id := roundID(p.GetRound(), p.GetPreviousSig())
	idx, _ := key.Scheme.IndexOf(p.GetPartialSig())
	round := c.getCache(id, p)
	if round == nil {
		return
	}
	if round.append(p) {
		// we increment the counter of that node index
		c.rcvd[idx] = append(c.rcvd[idx], id)
	}
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
func (c *partialCache) getCache(id string, p *drand.PartialBeaconPacket) *roundCache {
	if round, ok := c.rounds[id]; ok {
		return round
	}
	idx, _ := key.Scheme.IndexOf(p.GetPartialSig())
	if len(c.rcvd[idx]) >= MaxPartialsPerNode {
		// this node has submitted too many partials - we take the last one off
		toEvict := c.rcvd[idx][0]
		round, ok := c.rounds[toEvict]
		if !ok {
			c.l.Error("cache", "miss", "node", idx, "not_present_for", p.GetRound())
			return nil
		}
		round.flushIndex(idx)
		c.rcvd[idx] = append(c.rcvd[idx][1:], id)
		// if the round is now empty, delete it
		if round.Len() == 0 {
			delete(c.rounds, toEvict)
		}
	}
	round := newRoundCache(id, p)
	c.rounds[id] = round
	return round
}

type roundCache struct {
	round uint64
	prev  []byte
	id    string
	sigs  map[int][]byte
}

func newRoundCache(id string, p *drand.PartialBeaconPacket) *roundCache {
	return &roundCache{
		round: p.GetRound(),
		prev:  p.GetPreviousSig(),
		id:    id,
		sigs:  make(map[int][]byte),
	}
}

// append stores the partial and returns true if the partial is not stored . It
// returns false if the cache is already caching this partial signature.
func (r *roundCache) append(p *drand.PartialBeaconPacket) bool {
	idx, _ := key.Scheme.IndexOf(p.GetPartialSig())
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

// Msg provides the chain for the current round
func (r *roundCache) Msg() []byte {
	return chain.Message(r.round, r.prev)
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
