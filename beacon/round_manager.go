package beacon

import (
	"context"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/sign"
	clock "github.com/jonboulle/clockwork"
)

type roundManager struct {
	newRound   chan roundBundle
	newPartial chan *drand.PartialBeaconPacket
	stop       chan bool
	syncCh     chan roundPair
	expected   int
	sign       sign.ThresholdScheme
	l          log.Logger
	getheads   GetHeads
	clock      clock.Clock
}

type GetHeads func(context.Context) (int, chan *drand.BeaconPacket)

func newRoundManager(l log.Logger, c clock.Clock, thr int, gh GetHeads) *roundManager {
	r := &roundManager{
		newRound:   make(chan roundBundle, 1),
		newPartial: make(chan *drand.PartialBeaconPacket, thr),
		stop:       make(chan bool, 1),
		syncCh:     make(chan roundPair, 1),
		expected:   thr,
		sign:       key.Scheme,
		l:          l,
		getheads:   gh,
		clock:      c,
	}
	go r.run()
	return r
}

const maxLookAheadQueue = 1024

func (r *roundManager) run() {
	var currRound roundBundle
	var tmpPartials []*drand.PartialBeaconPacket
	var syncTick = r.clock.NewTicker(CheckSyncPeriod)
	defer syncTick.Stop()
	var highestRes = make(chan *drand.BeaconPacket, 1)
	var syncSignal bool
	for {
		select {
		case nRound := <-r.newRound:
			if currRound.partialCh != nil {
				// notify current round we moved on
				close(currRound.partialCh)
			}
			currRound = nRound
			syncSignal = false
			// we incorporate every tmp partials that corresponds to
			// this round and we flush every "old" partials or inconsistent
			// partials
			npartials := tmpPartials[:0]
			var toSend []*drand.PartialBeaconPacket
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
		case <-syncTick.Chan():
			// run a safety sync
			go func() {
				highestRes <- r.fetchBestHead()
			}()
		case bestSeen := <-highestRes:
			if r.checkIfSyncNeeded(currRound, bestSeen) && !syncSignal {
				syncSignal = true
				r.syncCh <- roundPair{
					previous: bestSeen.GetPreviousRound(),
					current:  bestSeen.GetRound(),
				}
			}
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

type beaconLike interface {
	GetPreviousRound() uint64
	GetRound() uint64
}

// this packet is already not for the current round but we look if it is an
// indicator that we should sync with others
func (r *roundManager) checkIfSyncNeeded(currRound roundBundle, p beaconLike) bool {
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

func (r *roundManager) fetchBestHead() *drand.BeaconPacket {
	ctx, cancel := context.WithTimeout(context.Background(), PeriodSyncTimeout)
	defer cancel()
	expected, respCh := r.getheads(ctx)
	var highest *drand.BeaconPacket
	var got int
	for {
		select {
		case beacon := <-respCh:
			got++
			if beacon != nil && highest == nil {
				highest = beacon
			} else if beacon != nil {
				highest = choice(highest, beacon)
			}
			if got == expected {
				return highest
			}
		case <-ctx.Done():
			return highest
		}
	}
}

func choice(b1, b2 *drand.BeaconPacket) *drand.BeaconPacket {
	if b1.GetRound() > b2.GetRound() {
		return b1
	} else if b2.GetRound() > b1.GetRound() {
		return b2
	}
	if b1.GetPreviousRound() > b2.GetPreviousRound() {
		return b1
	} else if b2.GetPreviousRound() > b1.GetPreviousRound() {
		return b2
	}
	return b1
}

type roundBundle struct {
	lastRound uint64
	round     uint64
	partialCh chan []byte
	seen      map[int]bool
}
