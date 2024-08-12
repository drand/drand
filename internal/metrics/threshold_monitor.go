package metrics

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/drand/drand/v2/common/log"
)

type ThresholdMonitor struct {
	lock              sync.RWMutex
	log               log.Logger
	beaconID          string
	groupSize         int
	threshold         int
	failedConnections map[string]bool
	ctx               context.Context
	cancel            func()
	period            time.Duration
}

func NewThresholdMonitor(beaconID string, l log.Logger, groupSize, threshold int) *ThresholdMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &ThresholdMonitor{
		lock:              sync.RWMutex{},
		log:               l,
		beaconID:          beaconID,
		groupSize:         groupSize,
		threshold:         threshold,
		failedConnections: make(map[string]bool),
		ctx:               ctx,
		cancel:            cancel,
		period:            1 * time.Minute,
	}
}

func (t *ThresholdMonitor) Start() {
	t.log.Infow("starting threshold monitor", "beaconID", t.beaconID)

	maxFailures := t.groupSize - t.threshold

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

				if len(failingNodes) >= maxFailures {
					t.log.Errorw(
						"failed connections crossed threshold in the last minute",
						"beaconID", t.beaconID,
						"groupSize", t.groupSize,
						"threshold", t.threshold,
						"failures", len(failingNodes),
						"nodes", strings.Join(failingNodes, ","),
					)
				} else if len(failingNodes) >= maxFailures/2 {
					t.log.Warnw(
						"failed connections crossed half threshold in the last minute",
						"beaconID", t.beaconID,
						"groupSize", t.groupSize,
						"threshold", t.threshold,
						"failures", len(failingNodes),
						"nodes", strings.Join(failingNodes, ","),
					)
				} else {
					t.log.Debugw(
						"threshold monitor healthy",
						"groupSize", t.groupSize,
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

func (t *ThresholdMonitor) ReportFailure(beaconID, addr string) {
	ErrorSendingPartial(beaconID, addr)
	t.lock.Lock()
	t.failedConnections[addr] = true
	t.lock.Unlock()
}

func (t *ThresholdMonitor) Update(newThreshold, groupSize int) {
	t.lock.Lock()
	t.threshold = newThreshold
	t.groupSize = groupSize
	t.lock.Unlock()
}
