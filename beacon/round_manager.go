package beacon

import (
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/sign"
)

type roundManager struct {
	newRound   chan roundBundle
	newPartial chan *drand.PartialBeaconPacket
	stop       chan bool
	syncCh     chan roundPair
	expected   int
	sign       sign.ThresholdScheme
	l          log.Logger
}

func newRoundManager(l log.Logger, thr int, s sign.ThresholdScheme) *roundManager {
	r := &roundManager{
		newRound:   make(chan roundBundle, 1),
		newPartial: make(chan *drand.PartialBeaconPacket, thr),
		stop:       make(chan bool, 1),
		syncCh:     make(chan roundPair, 1),
		expected:   thr,
		sign:       s,
		l:          l,
	}
	go r.run()
	return r
}

const maxLookAheadQueue = 1024

func (r *roundManager) run() {
	var currRound roundBundle
	var tmpPartials []*drand.PartialBeaconPacket
	for {
		select {
		case nRound := <-r.newRound:
			if currRound.partialCh != nil {
				// notify current round we moved on
				close(currRound.partialCh)
			}
			currRound = nRound
			// we incorporate every tmp partials that corresponds to
			// this round and we flush every "old" partials or inconsistent
			// partials
			npartials := tmpPartials[:0]
			var toSend []*drand.PartialBeaconPacket
			var syncSignal bool
			for _, partial := range tmpPartials {
				if r.checkIfForCurrent(currRound, partial) {
					// we have some partials for the round
					toSend = append(toSend, partial)
					continue
				}
				if !r.checkIfStoreForLater(currRound, partial) {
					// same round but invalid previous round, that means
					// it is invalid from this node point of view
					continue
				}
				npartials = append(npartials, partial)
				// we only check if sync is needed when we transition to a new
				// round because only then we know what is the new round from
				// this node's perspective.
				if r.checkIfSyncNeeded(currRound, partial) && !syncSignal {
					syncSignal = true
					r.syncCh <- roundPair{
						previous: partial.GetPreviousRound(),
						current:  partial.GetRound(),
					}
				}
			}
			tmpPartials = npartials
			if len(toSend) > 0 {
				go func() {
					r.l.Debug("round_manager", "future_partials", "push", len(toSend))
					for _, p := range toSend {
						r.newPartial <- p
					}
				}()

			}
		case partial := <-r.newPartial:
			if !r.checkIfForCurrent(currRound, partial) {
				if r.checkIfStoreForLater(currRound, partial) {
					// we are late behind what the other nodes are doing
					// we keep in the meantime until we are synced in
					if len(tmpPartials) > maxLookAheadQueue {
						// rotate the partials
						tmpPartials = tmpPartials[1:]
					}
					tmpPartials = append(tmpPartials, partial)
				}
				continue
			}
			index, _ := r.sign.IndexOf(partial.GetPartialSig())
			if seen := currRound.seen[index]; seen {
				r.l.Debug("round_manager", "seen_index", index, currRound.round)
				continue
			}
			currRound.seen[index] = true
			currRound.partialCh <- partial.GetPartialSig()
		case <-r.stop:
			return
		}
	}
}

func (r *roundManager) NewPartialBeacon(b *drand.PartialBeaconPacket) {
	r.newPartial <- b
}

func (r *roundManager) NewRound(prev, curr uint64) chan []byte {
	rb := roundBundle{
		lastRound: prev,
		round:     curr,
		partialCh: make(chan []byte, r.expected),
		seen:      make(map[int]bool),
	}
	r.newRound <- rb
	return rb.partialCh
}

func (r *roundManager) WaitSync() chan roundPair {
	return r.syncCh
}

type roundPair struct {
	previous uint64
	current  uint64
}

func (r *roundManager) checkIfForCurrent(currRound roundBundle, p *drand.PartialBeaconPacket) bool {
	sameRound := p.GetPreviousRound() == currRound.lastRound
	samePrevious := p.GetRound() == currRound.round
	return sameRound && samePrevious
}

// this packet is already not for the current round but we look if it is an
// indicator that we should sync with others
func (r *roundManager) checkIfSyncNeeded(currRound roundBundle, p *drand.PartialBeaconPacket) bool {
	if p.GetPreviousRound() > currRound.lastRound {
		r.l.Debug("round_manager", "invalid_previous", "want", currRound.lastRound, "got", p.GetPreviousRound(), "sync", "launch")
		return true
	}
	// if it just one round ahead, that means we are a bit late or node is a bit
	// in advance, and as soon as the next round kicks in we're gonna use that
	// partial. but if it is two round above, that means our clock is really
	// late & something's wrong - we should sync
	if p.GetRound() > currRound.round+1 {
		r.l.Debug("round_manager", "invalid_current", "want", currRound.round, "got", p.GetRound(), "sync", "launch", "check", "clock_drift")
		return true
	}
	return false
}

func (r *roundManager) checkIfStoreForLater(currRound roundBundle, p *drand.PartialBeaconPacket) bool {
	if p.GetRound() > currRound.round {
		return true
	}
	if p.GetPreviousRound() > currRound.lastRound {
		return true
	}
	return false
}

func (r *roundManager) Stop() {
	close(r.stop)
}

type roundBundle struct {
	lastRound uint64
	round     uint64
	partialCh chan []byte
	seen      map[int]bool
}
