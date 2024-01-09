package core

import (
	"time"


	clock "github.com/jonboulle/clockwork"

	"github.com/drand/drand/v2/internal/dkg"
	"github.com/drand/drand/v2/internal/net"
)

type MockNode struct {
	addr      string
	daemon    *DrandDaemon
	drand     *BeaconProcess
	clock     clock.FakeClock
	dkgRunner *dkg.TestRunner
}

// newNode creates a node struct from a daemon and sets the clock according to the test clock.
func newNode(now time.Time, daemon *DrandDaemon, dr *BeaconProcess) (*MockNode, error) {
	id := dr.priv.Public.Address()
	c := clock.NewFakeClockAt(now)

	// Note: not pure
	dr.opts.clock = c

	dkgClient, err := net.NewDKGControlClient(daemon.log, dr.opts.controlPort)
	if err != nil {
		return nil, err
	}

	return &MockNode{
		addr:   id,
		daemon: daemon,
		drand:  dr,
		clock:  c,
		dkgRunner: &dkg.TestRunner{
			BeaconID: dr.beaconID,
			Client:   dkgClient,
			Clock:    c,
		},
	}, nil
}

func (n *MockNode) GetAddr() string {
	return n.addr
}
