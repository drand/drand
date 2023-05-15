package metrics

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/drand/drand/common/log"
)

func TestLogsErrorsWhenThresholdReached(t *testing.T) {
	beaconID := "some-beacon"
	ctx, cancel := context.WithCancel(context.Background())
	l := &mockLogger{}
	threshold := 3
	period := 1 * time.Second
	monitor := ThresholdMonitor{
		lock:              sync.RWMutex{},
		log:               l,
		beaconID:          "default",
		threshold:         threshold,
		failedConnections: make(map[string]bool),
		ctx:               ctx,
		cancel:            cancel,
		period:            period,
	}

	l.On("Infow").Return()
	l.On("Errorw").Return()
	l.On("Debugw").Return()
	l.On("Warnw").Return()

	// starting monitor afterwards to avoid any flakiness on CI
	monitor.ReportFailure(beaconID, "a")
	monitor.ReportFailure(beaconID, "b")
	monitor.ReportFailure(beaconID, "c")
	monitor.Start()
	time.Sleep(period)
	monitor.Stop()

	l.AssertCalled(t, "Errorw", mock.Anything)
}

func TestLogsWarningsWhenThresholdAndAHalfReached(t *testing.T) {
	beaconID := "some-beacon"
	ctx, cancel := context.WithCancel(context.Background())
	l := &mockLogger{}
	threshold := 3
	period := 1 * time.Second
	monitor := ThresholdMonitor{
		lock:              sync.RWMutex{},
		log:               l,
		beaconID:          "default",
		threshold:         threshold,
		failedConnections: make(map[string]bool),
		ctx:               ctx,
		cancel:            cancel,
		period:            period,
	}

	l.On("Infow").Return()
	l.On("Errorw").Return()
	l.On("Debugw").Return()
	l.On("Warnw").Return()

	// starting monitor afterwards to avoid any flakiness on CI
	monitor.ReportFailure(beaconID, "a")
	monitor.ReportFailure(beaconID, "c")
	monitor.Start()
	time.Sleep(period)
	monitor.Stop()

	l.AssertCalled(t, "Warnw", mock.Anything)
	l.AssertNotCalled(t, "Errorw", mock.Anything)
}

func TestLogsDebugWhenAllGood(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	l := &mockLogger{}
	threshold := 3
	period := 1 * time.Second
	monitor := ThresholdMonitor{
		lock:              sync.RWMutex{},
		log:               l,
		beaconID:          "default",
		threshold:         threshold,
		failedConnections: make(map[string]bool),
		ctx:               ctx,
		cancel:            cancel,
		period:            period,
	}

	l.On("Infow").Return()
	l.On("Errorw").Return()
	l.On("Debugw").Return()
	l.On("Warnw").Return()

	monitor.Start()
	time.Sleep(period)
	monitor.Stop()

	l.AssertCalled(t, "Debugw", mock.Anything)
	l.AssertNotCalled(t, "Warnw", mock.Anything)
	l.AssertNotCalled(t, "Errorw", mock.Anything)
}

func TestStoppingMonitorStopsTheGoroutine(t *testing.T) {
	beaconID := "some-beacon"
	ctx, cancel := context.WithCancel(context.Background())
	l := &mockLogger{}
	threshold := 3
	period := 1 * time.Second
	monitor := ThresholdMonitor{
		lock:              sync.RWMutex{},
		log:               l,
		beaconID:          "default",
		threshold:         threshold,
		failedConnections: make(map[string]bool),
		ctx:               ctx,
		cancel:            cancel,
		period:            period,
	}

	l.On("Infow").Return()
	l.On("Errorw").Return()
	l.On("Debugw").Return()
	l.On("Warnw").Return()

	monitor.Start()
	monitor.Stop()
	monitor.ReportFailure(beaconID, "a")
	monitor.ReportFailure(beaconID, "b")
	monitor.ReportFailure(beaconID, "c")
	monitor.ReportFailure(beaconID, "d")
	time.Sleep(period)

	l.AssertNotCalled(t, "Debugw", mock.Anything)
	l.AssertNotCalled(t, "Warnw", mock.Anything)
	l.AssertNotCalled(t, "Errorw", mock.Anything)
}

func TestDuplicateFailuresAreOnlyCountedOnce(t *testing.T) {
	beaconID := "some-beacon"
	ctx, cancel := context.WithCancel(context.Background())
	l := &mockLogger{}
	threshold := 4
	period := 1 * time.Second
	monitor := ThresholdMonitor{
		lock:              sync.RWMutex{},
		log:               l,
		beaconID:          "default",
		threshold:         threshold,
		failedConnections: make(map[string]bool),
		ctx:               ctx,
		cancel:            cancel,
		period:            period,
	}

	l.On("Infow").Return()
	l.On("Errorw").Return()
	l.On("Debugw").Return()
	l.On("Warnw").Return()

	// starting monitor afterwards to avoid any flakiness on CI
	monitor.ReportFailure(beaconID, "a")
	monitor.ReportFailure(beaconID, "a")
	monitor.ReportFailure(beaconID, "a")
	monitor.ReportFailure(beaconID, "a")
	monitor.Start()
	time.Sleep(period)
	monitor.Stop()

	l.AssertCalled(t, "Debugw", mock.Anything)
	l.AssertNotCalled(t, "Warnw", mock.Anything)
	l.AssertNotCalled(t, "Errorw", mock.Anything)
}

func TestStateIsResetEveryPeriod(t *testing.T) {
	beaconID := "some-beacon"
	ctx, cancel := context.WithCancel(context.Background())
	l := &mockLogger{}
	threshold := 3
	period := 1 * time.Second
	monitor := ThresholdMonitor{
		lock:              sync.RWMutex{},
		log:               l,
		beaconID:          "default",
		threshold:         threshold,
		failedConnections: make(map[string]bool),
		ctx:               ctx,
		cancel:            cancel,
		period:            period,
	}

	l.On("Infow").Return()
	l.On("Errorw").Return()
	l.On("Debugw").Return()
	l.On("Warnw").Return()

	// starting monitor afterwards to avoid any flakiness on CI
	monitor.ReportFailure(beaconID, "a")
	monitor.Start()
	time.Sleep(period)
	monitor.ReportFailure(beaconID, "b")
	time.Sleep(period)
	monitor.Stop()

	l.AssertCalled(t, "Warnw", mock.Anything)
	l.AssertNotCalled(t, "Errorw", mock.Anything)
}

type mockLogger struct {
	mock.Mock
}

func (m *mockLogger) Info(keyvals ...interface{}) {
	panic("implement me")
}

func (m *mockLogger) Debug(keyvals ...interface{}) {
	panic("implement me")
}

func (m *mockLogger) Warn(keyvals ...interface{}) {
	panic("implement me")
}

func (m *mockLogger) Error(keyvals ...interface{}) {
	panic("implement me")
}

func (m *mockLogger) Fatal(keyvals ...interface{}) {
	panic("implement me")
}

func (m *mockLogger) Panic(keyvals ...interface{}) {
	panic("implement me")
}

func (m *mockLogger) Infow(msg string, keyvals ...interface{}) {
	m.Called()
}

func (m *mockLogger) Debugw(msg string, keyvals ...interface{}) {
	m.Called()
}

func (m *mockLogger) Warnw(msg string, keyvals ...interface{}) {
	m.Called()
}

func (m *mockLogger) Errorw(msg string, keyvals ...interface{}) {
	m.Called()
}

func (m *mockLogger) Fatalw(msg string, keyvals ...interface{}) {
	panic("implement me")
}

func (m *mockLogger) Panicw(msg string, keyvals ...interface{}) {
	panic("implement me")
}

func (m *mockLogger) With(args ...interface{}) log.Logger {
	panic("implement me")
}

func (m *mockLogger) Named(s string) log.Logger {
	panic("implement me")
}

func (m *mockLogger) AddCallerSkip(skip int) log.Logger {
	panic("implement me")
}
