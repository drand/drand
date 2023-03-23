package core

import (
	"time"

	clock "github.com/jonboulle/clockwork"

	"github.com/drand/drand/net"
	"github.com/drand/drand/test"
)

type MockNode struct {
	addr      string
	certPath  string
	daemon    *DrandDaemon
	drand     *BeaconProcess
	clock     clock.FakeClock
	dkgRunner *test.DKGRunner
}

// newNode creates a node struct from a drand and sets the clock according to the drand test clock.
func newNode(now time.Time, certPath string, daemon *DrandDaemon, dr *BeaconProcess) (*MockNode, error) {
	id := dr.priv.Public.Address()
	c := clock.NewFakeClockAt(now)

	// Note: not pure
	dr.opts.clock = c

	dkgClient, err := net.NewDKGControlClientWithLogger(daemon.log, dr.opts.controlPort)
	if err != nil {
		return nil, err
	}

	return &MockNode{
		certPath: certPath,
		addr:     id,
		daemon:   daemon,
		drand:    dr,
		clock:    c,
		dkgRunner: &test.DKGRunner{
			BeaconID: dr.beaconID,
			Client:   dkgClient,
		},
	}, nil
}

func (n *MockNode) GetAddr() string {
	return n.addr
}
