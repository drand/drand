package metrics

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/drand/drand/log"
)

type ThresholdMonitor struct {
	lock              sync.RWMutex
	log               log.Logger
	beaconID          string
	threshold         int
	failedConnections map[string]bool
	ctx               context.Context
	cancel            func()
	period            time.Duration
}

func NewThresholdMonitor(beaconID string, l log.Logger, threshold int) *ThresholdMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &ThresholdMonitor{
		lock:              sync.RWMutex{},
		log:               l,
		beaconID:          beaconID,
		threshold:         threshold,
		failedConnections: make(map[string]bool),
		ctx:               ctx,
		cancel:            cancel,
		period:            1 * time.Minute,
	}
}

func (t *ThresholdMonitor) Start() {
	t.log.Infow("starting threshold monitor", "beaconID", t.beaconID)

	go func() {
		for {
			select {
			case <-t.ctx.Done():
				t.log.Infow("ending threshold monitor", "beaconID", t.beaconID)
				return
			default:
				t.lock.RLock()
				var failingNodes []string
				for address := range t.failedConnections {
					failingNodes = append(failingNodes, address)
				}

				if len(failingNodes) >= t.threshold {
					t.log.Errorw(
						"failed connections crossed threshold in the last minute",
						"beaconID", t.beaconID,
						"threshold", t.threshold,
						"failures", len(failingNodes),
						"nodes", strings.Join(failingNodes, ","),
					)
				} else if len(failingNodes) >= t.threshold/2 {
					t.log.Warnw(
						"failed connections crossed half threshold in the last minute",
						"beaconID", t.beaconID,
						"threshold", t.threshold,
						"failures", len(failingNodes),
						"nodes", strings.Join(failingNodes, ","),
					)
				} else {
					t.log.Debugw(
						"threshold monitor healthy",
						"threshold", t.threshold,
						"beaconID", t.beaconID,
						"failures", len(failingNodes),
						"nodes", strings.Join(failingNodes, ","),
					)
				}
				t.lock.RUnlock()

				t.lock.Lock()
				t.failedConnections = make(map[string]bool)
				t.lock.Unlock()

				time.Sleep(t.period)
			}
		}
	}()
}

func (t *ThresholdMonitor) Stop() {
	t.cancel()
}

func (t *ThresholdMonitor) ReportFailure(beaconID string, round uint64, addr string) {
	t.lock.Lock()
	t.failedConnections[addr] = true
	t.lock.Unlock()
	ErrorSendingPartial(beaconID, round, addr)
}

func (t *ThresholdMonitor) UpdateThreshold(newThreshold int) {
	t.lock.Lock()
	t.threshold = newThreshold
	t.lock.Unlock()
}
