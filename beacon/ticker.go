package beacon

import (
	"time"

	clock "github.com/jonboulle/clockwork"
)

type ticker struct {
	clock   clock.Clock
	period  time.Duration
	genesis int64
	newCh   chan chan roundInfo
	stop    chan bool
}

func newTicker(c clock.Clock, period time.Duration, genesis int64) *ticker {
	t := &ticker{
		clock:   c,
		period:  period,
		genesis: genesis,
		newCh:   make(chan chan roundInfo, 5),
		stop:    make(chan bool, 1),
	}
	go t.Start()
	return t
}

func (t *ticker) Channel() chan roundInfo {
	newCh := make(chan roundInfo, 1)
	t.newCh <- newCh
	return newCh
}

func (t *ticker) Stop() {
	close(t.stop)
}

func (t *ticker) CurrentRound() uint64 {
	return CurrentRound(t.clock.Now().Unix(), t.period, t.genesis)
}

func (t *ticker) Start() {
	ticker := t.clock.NewTicker(t.period)
	var channels []chan roundInfo
	tickChan := ticker.Chan()
	for {
		select {
		case nt := <-tickChan:
			round, time := NextRound(nt.Unix(), t.period, t.genesis)
			info := roundInfo{
				round: round - 1,
				time:  time - int64(t.period.Seconds()),
			}
			for _, ch := range channels {
				ch <- info
			}
		case newChan := <-t.newCh:
			channels = append(channels, newChan)
		case <-t.stop:
			ticker.Stop()
			for _, ch := range channels {
				close(ch)
			}
			return
		}
	}
}

type roundInfo struct {
	round uint64
	time  int64
}
