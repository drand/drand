package core

import (
	"sync"

	"github.com/drand/drand/beacon"
)

type callbackManager struct {
	sync.Mutex
	callbacks map[string]func(*beacon.Beacon)
	stop      chan bool
	newCb     chan callback
}

const streamRoutines int = 5

func newCallbackManager() *callbackManager {
	s := &callbackManager{
		callbacks: make(map[string]func(*beacon.Beacon)),
		newCb:     make(chan callback, 100),
		stop:      make(chan bool),
	}
	for i := 0; i < streamRoutines; i++ {
		go s.runWorker()
	}
	return s
}

// AddCallback stores the given callbacks. It will be called for each incoming
// beacon. If callbacks already exists, it is overwritten.
func (s *callbackManager) AddCallback(id string, fn func(*beacon.Beacon)) {
	s.Lock()
	defer s.Unlock()
	s.callbacks[id] = fn
}

func (s *callbackManager) DelCallback(id string) {
	s.Lock()
	defer s.Unlock()
	delete(s.callbacks, id)

}

func (s *callbackManager) NewBeacon(b *beacon.Beacon) {
	s.Lock()
	defer s.Unlock()
	for _, cb := range s.callbacks {
		s.newCb <- callback{
			beacon: b,
			cb:     cb,
		}
	}
}

type callback struct {
	cb     func(*beacon.Beacon)
	beacon *beacon.Beacon
}

func (s *callbackManager) runWorker() {
	for {
		select {
		case cbd := <-s.newCb:
			cbd.cb(cbd.beacon)
		case <-s.stop:
			return
		}
	}
}
