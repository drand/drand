//nolint:dupl
package regression

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/internal/chain"
	"github.com/drand/drand/v2/internal/core"
	"github.com/drand/drand/v2/internal/dkg"
	"github.com/drand/drand/v2/internal/net"
	"github.com/drand/drand/v2/internal/test"
	dkgproto "github.com/drand/drand/v2/protobuf/dkg"
	"github.com/drand/drand/v2/protobuf/drand"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
)

const oldBinaryPath = "./drand-1.5.7"

// node is a convenience wrapper around some state to avoid rebinding ports
type node struct {
	private string
	public  string
	control string
	dir     string
}

func NewNode(t *testing.T) *node {
	return &node{
		private: fmt.Sprintf("localhost:%s", test.FreePort()),
		public:  fmt.Sprintf("localhost:%s", test.FreePort()),
		control: test.FreePort(),
		dir:     t.TempDir(),
	}
}

//nolint:funlen
func TestMigrateOldGroupFile(t *testing.T) {
	n := 3

	// we create a few nodes on v1.5.7
	nodes := make([]*node, n)
	for i := 0; i < n; i++ {
		i := i
		node := NewNode(t)
		nodes[i] = node
		folderFlag := fmt.Sprintf("--folder=%s", node.dir)
		controlFlag := fmt.Sprintf("--control=%s", node.control)
		require.NoError(t, runCommand(
			oldBinaryPath,
			"generate-keypair",
			"--id=default",
			"--scheme=pedersen-bls-chained",
			"--insecure",
			controlFlag,
			folderFlag,
			node.private,
		),
		)
		go func() {
			require.NoError(t, runCommand(
				oldBinaryPath,
				"start",
				"--insecure",
				fmt.Sprintf("--private-listen=%s", node.private),
				fmt.Sprintf("--public-listen=%s", node.public),
				controlFlag,
				folderFlag,
			),
			)
		}()
	}

	// wait for all the nodes to start
	for i := 0; i < n; i++ {
		require.NoError(t, waitForNodeStarted(nodes[i].public, 20))
	}

	// we run a DKG on them
	go func() {
		_ = runCommand(
			oldBinaryPath,
			"share",
			"--leader",
			"--threshold=2",
			"--nodes=3",
			fmt.Sprintf("--control=%s", nodes[0].control),
			"--period=3s",
			"--catchup-period=1s",
			"--secret-file=secretfile",
			"--insecure",
		)
	}()

	// leave some time for the leader to do its thing
	time.Sleep(1 * time.Second)

	for _, n := range nodes[1:] {
		n := n
		go func() {
			_ = runCommand(
				oldBinaryPath,
				"share",
				fmt.Sprintf("--connect=%s", nodes[0].private),
				fmt.Sprintf("--control=%s", n.control),
				"--secret-file=secretfile", "--insecure")
		}()
	}

	// wait for all the nodes to finish the DKG
	for i := 0; i < n; i++ {
		require.NoError(t, waitForDKGComplete(nodes[i].control, 60))
	}

	// now we start the nodes on v2, reusing the folders from v1.5.7
	daemons := make([]*core.DrandDaemon, n)
	for i := 0; i < n; i++ {
		// stopping the existing daemon actually returns an error, so we ignore it fuuu
		_ = runCommand(oldBinaryPath, "stop", fmt.Sprintf("--control=%s", nodes[i].control))

		// we have to self sign the keys, as the CLI normally does this for us
		_, _, err := core.SelfSignKeys(log.DefaultLogger(), fmt.Sprintf("%s/multibeacon", nodes[i].dir))
		require.NoError(t, err)

		opts := []core.ConfigOption{
			core.WithConfigFolder(nodes[i].dir),
			core.WithPrivateListenAddress(nodes[i].private),
			core.WithPublicListenAddress(nodes[i].public),
			core.WithControlPort(nodes[i].control),
			core.WithDBStorageEngine(chain.BoltDB),
		}
		conf := core.NewConfig(log.DefaultLogger(), opts...)
		d, err := core.NewDrandDaemon(context.Background(), conf)
		require.NoError(t, err)
		require.NoError(t, d.LoadBeaconsFromDisk(context.Background(), "", false, "default"))

		daemons[i] = d
	}

	// wait for all the nodes to start
	for i := 0; i < n; i++ {
		require.NoError(t, waitForNodeStarted(nodes[i].public, 20))
	}

	// now that the daemons are started, we extract their identities and set up some plumbing for the DKG
	identities := make([]*dkgproto.Participant, n)
	runners := make([]*dkg.TestRunner, n)
	for i := 0; i < n; i++ {
		client, err := net.NewDKGControlClient(log.DefaultLogger(), nodes[i].control)
		require.NoError(t, err)
		runners[i] = &dkg.TestRunner{
			Client:   client,
			BeaconID: "default",
			Clock:    clockwork.NewRealClock(),
		}

		ident, err := daemons[i].GetIdentity(context.Background(), &drand.IdentityRequest{Metadata: &drand.Metadata{BeaconID: "default"}})
		require.NoError(t, err)
		identities[i] = &dkgproto.Participant{
			Address:   ident.Address,
			Key:       ident.Key,
			Signature: ident.Signature,
		}
	}

	// then we actually do the reshare
	require.NoError(t, runners[0].StartReshare(2, 1, nil, identities, nil))
	require.NoError(t, runners[1].Accept())
	require.NoError(t, runners[2].Accept())
	require.NoError(t, runners[0].StartExecution())
	require.NoError(t, runners[0].WaitForDKG(log.DefaultLogger(), 2, 60))
}

//nolint:funlen
func TestLeaverNodeDownDoesntFailProposal(t *testing.T) {
	n := 3

	// we create a few nodes on v1.5.7
	nodes := make([]*node, n)
	for i := 0; i < n; i++ {
		i := i
		node := NewNode(t)
		nodes[i] = node
		folderFlag := fmt.Sprintf("--folder=%s", node.dir)
		controlFlag := fmt.Sprintf("--control=%s", node.control)
		require.NoError(t, runCommand(
			oldBinaryPath,
			"generate-keypair",
			"--id=default",
			"--scheme=pedersen-bls-chained",
			"--insecure",
			controlFlag,
			folderFlag,
			node.private,
		),
		)
		go func() {
			require.NoError(t, runCommand(
				oldBinaryPath,
				"start",
				"--insecure",
				fmt.Sprintf("--private-listen=%s", node.private),
				fmt.Sprintf("--public-listen=%s", node.public),
				controlFlag,
				folderFlag,
			),
			)
		}()
	}

	// wait for all the nodes to start
	for i := 0; i < n; i++ {
		require.NoError(t, waitForNodeStarted(nodes[i].public, 20))
	}

	// we run a DKG on them
	go func() {
		_ = runCommand(
			oldBinaryPath,
			"share",
			"--leader",
			"--threshold=2",
			"--nodes=3",
			fmt.Sprintf("--control=%s", nodes[0].control),
			"--period=3s",
			"--catchup-period=1s",
			"--secret-file=secretfile",
			"--insecure",
		)
	}()

	// wait some time for the leader to do its thing
	time.Sleep(1 * time.Second)

	for _, n := range nodes[1:] {
		n := n
		go func() {
			_ = runCommand(
				oldBinaryPath,
				"share",
				fmt.Sprintf("--connect=%s", nodes[0].private),
				fmt.Sprintf("--control=%s", n.control),
				"--secret-file=secretfile", "--insecure")
		}()
	}

	// wait for all the nodes to finish the DKG
	for i := 0; i < n; i++ {
		require.NoError(t, waitForDKGComplete(nodes[i].control, 60))
	}

	// now we only run two nodes for the reshare, and the third node becomes a leaver
	for i := 0; i < n; i++ {
		// stopping the existing daemon actually returns an error, so we ignore it fuuu
		_ = runCommand(oldBinaryPath, "stop", fmt.Sprintf("--control=%s", nodes[i].control))
	}

	newN := 2
	// we start the nodes on v2, reusing the folders from v1.5.7
	daemons := make([]*core.DrandDaemon, n)
	for i := 0; i < newN; i++ {
		// we have to self sign the keys, as the CLI normally does this for us
		_, _, err := core.SelfSignKeys(log.DefaultLogger(), fmt.Sprintf("%s/multibeacon", nodes[i].dir))
		require.NoError(t, err)

		opts := []core.ConfigOption{
			core.WithConfigFolder(nodes[i].dir),
			core.WithPrivateListenAddress(nodes[i].private),
			core.WithPublicListenAddress(nodes[i].public),
			core.WithControlPort(nodes[i].control),
			core.WithDBStorageEngine(chain.BoltDB),
		}
		conf := core.NewConfig(log.DefaultLogger(), opts...)
		d, err := core.NewDrandDaemon(context.Background(), conf)
		require.NoError(t, err)
		require.NoError(t, d.LoadBeaconsFromDisk(context.Background(), "", false, "default"))

		daemons[i] = d
	}

	// wait for all the nodes to start
	for i := 0; i < newN; i++ {
		require.NoError(t, waitForNodeStarted(nodes[i].public, 20))
	}

	// we pull the list of participants from last epoch
	status, err := daemons[0].DKGStatus(context.Background(), &dkgproto.DKGStatusRequest{BeaconID: "default"})
	require.NoError(t, err)
	// and get the leaver - their key won't be available to create the next proposal
	leavingNode := nodes[n-1]
	var leaver *dkgproto.Participant
	for _, participant := range status.Complete.Joining {
		// the addresses start from 0, so n-1

		if participant.Address == leavingNode.private {
			leaver = participant
		}
	}

	// now that the daemons are started, we extract their identities and set up some plumbing for the DKG
	runners := make([]*dkg.TestRunner, newN)
	remainers := make([]*dkgproto.Participant, newN)
	for i := 0; i < newN; i++ {
		client, err := net.NewDKGControlClient(log.DefaultLogger(), nodes[i].control)
		require.NoError(t, err)
		runners[i] = &dkg.TestRunner{
			Client:   client,
			BeaconID: "default",
			Clock:    clockwork.NewRealClock(),
		}

		ident, err := daemons[i].GetIdentity(context.Background(), &drand.IdentityRequest{Metadata: &drand.Metadata{BeaconID: "default"}})
		require.NoError(t, err)
		remainers[i] = &dkgproto.Participant{
			Address:   ident.Address,
			Key:       ident.Key,
			Signature: ident.Signature,
		}
	}

	// then we actually do the reshare without node 3
	require.NoError(t, runners[0].StartReshare(2, 1, nil, remainers, []*dkgproto.Participant{leaver}))
	require.NoError(t, runners[1].Accept())
	require.NoError(t, runners[0].StartExecution())
	require.NoError(t, runners[0].WaitForDKG(log.DefaultLogger(), 2, 60))
}

func runCommand(cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func waitForNodeStarted(publicAddress string, seconds int) error {
	var err error
	remaining := seconds
	for remaining > 0 {
		res, err := http.Get(fmt.Sprintf("http://%s/health", publicAddress))
		if err == nil || (res != nil && res.StatusCode != 200) {
			break
		}
		remaining--
		time.Sleep(1 * time.Second)
	}

	return err
}

func waitForDKGComplete(controlPort string, seconds int) error {
	var err error
	remaining := seconds
	for remaining > 0 {
		err = runCommand(oldBinaryPath, "show", "group", fmt.Sprintf("--control=%s", controlPort), "--id=default")
		if err == nil {
			break
		}
		remaining--
		time.Sleep(1 * time.Second)
	}

	return err
}
