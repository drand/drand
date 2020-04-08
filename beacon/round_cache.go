package beacon

import (
	"strconv"

	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/sign"
)

type roundManager struct {
	newRound  chan roundBundle
	newBeacon chan *drand.BeaconPacket
	stop      chan bool
	expected  int
	sign      sign.ThresholdScheme
	l         log.Logger
}

func newRoundManager(l log.Logger, thr int, s sign.ThresholdScheme) *roundManager {
	r := &roundManager{
		newRound:  make(chan roundBundle, 1),
		newBeacon: make(chan *drand.BeaconPacket, thr),
		stop:      make(chan bool, 1),
		expected:  thr,
		sign:      s,
		l:         l,
	}
	go r.run()
	return r
}

const maxLookAheadQueue = 1024

func (r *roundManager) run() {
	var currRound roundBundle
	var seenPartials = make(map[int]bool)
	var tmpPartials []*drand.BeaconPacket

	checkPartial := func(p *drand.BeaconPacket) bool {
		nowPrevious := p.GetPreviousRound() == currRound.lastRound
		if !nowPrevious {
			msgs := []string{"check_for", "sync"}
			if p.GetPreviousRound() < currRound.lastRound {
				msgs[0] = "late_node_diff"
				msgs[1] = strconv.Itoa(int(currRound.lastRound - p.GetPreviousRound()))
			}
			r.l.Debug("round_manager", "invalid_previous", "want", currRound.lastRound, "got", p.GetPreviousRound(), msgs[0], msgs[1])
			return false
		}
		notCurrent := p.GetRound() != currRound.round
		if notCurrent {
			r.l.Debug("round_manager", "not_current_round", "want", currRound.round, "got", p.GetRound(), "check_for", "late_time")
			return false
		}
		return true
	}
	for {
		select {
		case nRound := <-r.newRound:
			if currRound.partialCh != nil {
				close(currRound.partialCh)
			}
			currRound = nRound
			seenPartials = make(map[int]bool)
			// we incorporate every tmp partials that corresponds to
			// this round and we flush every "old" partials or inconsistent
			// partials
			npartials := tmpPartials[:0]
			var toSend []*drand.BeaconPacket
			for _, partial := range tmpPartials {
				if checkPartial(partial) {
					// we have some partials for the round
					toSend = append(toSend, partial)
					continue
				}
				if partial.GetRound() == currRound.round {
					// same round but invalid previous round, that means
					// it is invalid from this node point of view
					continue
				}
				if partial.GetRound() > currRound.round {
					// a futur round although that shouldn't happen
					// i.e. delta should be 1 at most since beacon only insert
					// "current rounds"
					npartials = append(npartials, partial)
				}
			}
			tmpPartials = npartials
			if len(toSend) > 0 {
				go func() {
					r.l.Debug("round_manager", "future_partials", "push", len(toSend))
					for _, p := range toSend {
						r.newBeacon <- p
					}
				}()

			}
		case partial := <-r.newBeacon:
			if !checkPartial(partial) {
				// if not for the current round this node thinks it is,
				// then look if we can store it for later
				if partial.GetRound() > currRound.round {
					// we are late behind what the other nodes are doing
					// we keep in the meantime until we are synced in
					if len(tmpPartials) < maxLookAheadQueue {
						tmpPartials = append(tmpPartials, partial)
					}
					continue
				}
			}

			index, _ := r.sign.IndexOf(partial.GetPartialSig())
			if seen := seenPartials[index]; seen {
				r.l.Debug("round_manager", "seen_index", index, currRound.round)
				continue
			}
			seenPartials[index] = true
			currRound.partialCh <- partial.GetPartialSig()
		case <-r.stop:
			return
		}
	}
}

func (r *roundManager) NewBeacon(b *drand.BeaconPacket) {
	r.newBeacon <- b
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

func (r *roundManager) Stop() {
	close(r.stop)
}

type roundBundle struct {
	lastRound uint64
	round     uint64
	partialCh chan []byte
	seen      map[int]bool
}
