package dkg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/net"
	"github.com/drand/drand/v2/internal/util"
	"github.com/drand/drand/v2/protobuf/dkg"

	clock "github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

//nolint:funlen // it's a test
func TestDKGFailedAtProtocol(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	beaconID := "default"
	nodeCount := 3
	mb := newMessageBus()

	nodes := make([]*stubbedDKGProcess, nodeCount)
	identities := make([]*dkg.Participant, nodeCount)
	for i := 0; i < nodeCount; i++ {
		stub, err := newStubbedDKGProcess(t, fmt.Sprintf("a:888%d", i), mb, beaconID)
		require.NoError(t, err)
		identity, err := util.PublicKeyAsParticipant(stub.key.Public)
		require.NoError(t, err)

		nodes[i] = stub
		identities[i] = identity
	}

	// the leader kicks off a DKG
	leaderNode := nodes[0]
	leader, err := leaderNode.RunnerFor(beaconID)
	require.NoError(t, err)
	err = leader.StartNetwork(2, 1, crypto.DefaultSchemeID, 2*time.Minute, 1, identities)
	require.NoError(t, err)

	// the nodes all join, but immediately go down
	for _, n := range nodes[1:] {
		r, err := n.RunnerFor(beaconID)
		require.NoError(t, err)
		err = r.JoinDKG()
		require.NoError(t, err)
		n.Break()
	}

	// the leader kicks off execution
	err = leader.StartExecution()
	require.NoError(t, err)

	// we wait some time for the DKG to fail
	err = leader.WaitForDKG(log.DefaultLogger(), 1, 30)
	require.Error(t, ErrDKGFailed, err)

	// we then check there are still no 'completed' DKGs
	failedStatus, err := leaderNode.DKGStatus(context.Background(), &dkg.DKGStatusRequest{BeaconID: beaconID})
	require.NoError(t, err)
	require.Equal(t, Failed.String(), Status(failedStatus.Current.State).String())
	require.Nil(t, failedStatus.Complete)

	// we call abort on each of the failed nodes, and recover them
	for _, n := range nodes[1:] {
		r, err := n.RunnerFor(beaconID)
		require.NoError(t, err)
		err = r.Abort()
		require.NoError(t, err)
		n.Fix()
	}

	// the leader kicks off the DKG again
	err = leader.StartNetwork(2, 1, crypto.DefaultSchemeID, 2*time.Minute, 1, identities)
	require.NoError(t, err)

	// this time each node joins without error
	for _, n := range nodes[1:] {
		r, err := n.RunnerFor(beaconID)
		require.NoError(t, err)
		err = r.JoinDKG()
		require.NoError(t, err)
	}

	// the leader kicks off execution
	err = leader.StartExecution()
	require.NoError(t, err)

	// and after a short pause
	err = leader.WaitForDKG(log.DefaultLogger(), 1, 30)
	require.NoError(t, err)

	// the leader reports the DKG is complete
	successfulStatus, err := leaderNode.DKGStatus(context.Background(), &dkg.DKGStatusRequest{BeaconID: beaconID})
	require.NoError(t, err)
	require.Equal(t, Complete.String(), Status(successfulStatus.Current.State).String())
	require.Equal(t, Complete.String(), Status(successfulStatus.Complete.State).String())
}

func TestFailedReshare(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	beaconID := "default"
	nodeCount := 3
	mb := newMessageBus()

	nodes := make([]*stubbedDKGProcess, nodeCount)
	identities := make([]*dkg.Participant, nodeCount)
	for i := 0; i < nodeCount; i++ {
		stub, err := newStubbedDKGProcess(t, fmt.Sprintf("a:888%d", i), mb, beaconID)
		require.NoError(t, err)
		identity, err := util.PublicKeyAsParticipant(stub.key.Public)
		require.NoError(t, err)

		nodes[i] = stub
		identities[i] = identity
	}

	leaderNode := nodes[0]
	leader, err := leaderNode.RunnerFor(beaconID)
	require.NoError(t, err)
	err = leader.StartNetwork(2, 1, crypto.DefaultSchemeID, 1*time.Minute, 1, identities)
	require.NoError(t, err)

	for _, n := range nodes[1:] {
		r, err := n.RunnerFor(beaconID)
		require.NoError(t, err)
		err = r.JoinDKG()
		require.NoError(t, err)
	}

	err = leader.StartExecution()
	require.NoError(t, err)

	err = leader.WaitForDKG(log.DefaultLogger(), 1, 60)
	require.NoError(t, err)

	err = leader.StartReshare(2, 1, nil, identities, nil)
	require.NoError(t, err)

	for _, n := range nodes[1:] {
		r, err := n.RunnerFor(beaconID)
		require.NoError(t, err)
		err = r.Accept()
		require.NoError(t, err)
	}

	err = leader.StartExecution()
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	for _, n := range nodes[1:] {
		n.Break()
	}

	err = leader.WaitForDKG(log.DefaultLogger(), 2, 60)
	require.Error(t, ErrDKGFailed, err)
	status, err := leaderNode.delegate.DKGStatus(context.Background(), &dkg.DKGStatusRequest{BeaconID: beaconID})
	require.NoError(t, err)
	require.Equal(t, uint32(1), status.Complete.Epoch)
}

//nolint:funlen // it's a test
func TestMultipleDKGsInFlight(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	beaconIDs := []string{"default", "other", "another", "fourth"}
	nodeCount := 3
	mb := newMessageBus()

	nodes := make([]*stubbedDKGProcess, nodeCount)
	identities := make([]*dkg.Participant, nodeCount)
	for i := 0; i < nodeCount; i++ {
		stub, err := newStubbedDKGProcess(t, fmt.Sprintf("a:888%d", i), mb, beaconIDs...)
		require.NoError(t, err)
		identity, err := util.PublicKeyAsParticipant(stub.key.Public)
		require.NoError(t, err)

		nodes[i] = stub
		identities[i] = identity
	}

	leaderNode := nodes[0]
	wg := sync.WaitGroup{}
	wg.Add(len(beaconIDs))
	// this time we start a DKG for every beaconID
	for _, beaconID := range beaconIDs {
		go func(beaconID string) {
			leader, err := leaderNode.RunnerFor(beaconID)
			require.NoError(t, err)

			dkgCompletionChannel := leaderNode.delegate.completedDKGs.Listen()

			err = leader.StartNetwork(2, 1, crypto.DefaultSchemeID, 1*time.Minute, 1, identities)
			require.NoError(t, err)

			for _, n := range nodes[1:] {
				r, err := n.RunnerFor(beaconID)
				require.NoError(t, err)
				err = r.JoinDKG()
				require.NoError(t, err)
			}

			err = leader.StartExecution()
			require.NoError(t, err)

			// we then wait for the signal on the completion channel that the DKG has been completed for this beaconID
			for {
				result := <-dkgCompletionChannel
				if result.BeaconID != beaconID {
					continue
				}
				break
			}

			// we then run a resharing
			err = leader.StartReshare(2, 1, nil, identities, nil)
			require.NoError(t, err)

			for _, n := range nodes[1:] {
				r, err := n.RunnerFor(beaconID)
				require.NoError(t, err)
				err = r.Accept()
				require.NoError(t, err)
			}

			err = leader.StartExecution()
			require.NoError(t, err)

			// then we wait for the signal on the completion channel that the DKG has been completed for this beaconID
			for {
				result := <-dkgCompletionChannel
				if result.BeaconID != beaconID {
					continue
				}
				break
			}
			wg.Done()
		}(beaconID)
	}

	wg.Wait()
}

func TestAbortedDKGCanRestart(t *testing.T) {
	// first we do a DKG with all joiners
	// then we change some participants around, start a DKG but abort it
	// then we try again but with some newer participants again and let the DKG finish

	beaconID := "default"
	nodeCount := 3
	mb := newMessageBus()

	// first we create some nodes
	nodes := make([]*stubbedDKGProcess, nodeCount)
	identities := make([]*dkg.Participant, nodeCount)
	for i := 0; i < nodeCount; i++ {
		stub, err := newStubbedDKGProcess(t, fmt.Sprintf("a:888%d", i), mb, beaconID)
		require.NoError(t, err)
		identity, err := util.PublicKeyAsParticipant(stub.key.Public)
		require.NoError(t, err)

		nodes[i] = stub
		identities[i] = identity
	}

	// then we run the initial DKG
	leader, err := nodes[0].RunnerFor(beaconID)
	require.NoError(t, err)
	err = leader.StartNetwork(2, 1, crypto.DefaultSchemeID, 1*time.Minute, 1, identities)
	require.NoError(t, err)
	for _, node := range nodes[1:] {
		r, err := node.RunnerFor(beaconID)
		require.NoError(t, err)
		require.NoError(t, r.JoinDKG())
	}
	require.NoError(t, leader.StartExecution())

	// we do some magic to get the group file for the new joiner in the next epoch
	dkgResult := <-nodes[0].delegate.completedDKGs.Listen()
	firstEpochGroup := dkgResult.New.FinalGroup
	require.NoError(t, leader.WaitForDKG(log.DefaultLogger(), 1, 60))

	// we create a new joiner
	newJoiner, err := newStubbedDKGProcess(t, fmt.Sprintf("a:888%d", 3), mb, beaconID)
	require.NoError(t, err)
	newKey, err := util.PublicKeyAsParticipant(newJoiner.key.Public)
	require.NoError(t, err)

	// then start a resharing
	err = leader.StartReshare(3, 1, []*dkg.Participant{newKey}, identities, nil)
	require.NoError(t, err)
	// but quickly abort it
	err = leader.Abort()
	require.NoError(t, err)

	// now we try the same ceremony again
	err = leader.StartReshare(3, 1, []*dkg.Participant{newKey}, identities, nil)
	require.NoError(t, err)

	for _, node := range nodes[1:] {
		r, err := node.RunnerFor(beaconID)
		require.NoError(t, err)
		require.NoError(t, r.Accept())
	}

	// the new joiner passes the old group file to join
	r, err := newJoiner.RunnerFor(beaconID)
	require.NoError(t, err)
	require.NoError(t, r.JoinReshare(firstEpochGroup))

	// we complete the DKG successfully
	require.NoError(t, leader.StartExecution())
	require.NoError(t, leader.WaitForDKG(log.DefaultLogger(), 2, 60))
}

func TestFailedFirstEpochCanRecover(t *testing.T) {
	// we do an initial DKG, but abort
	// then retry (still at epoch 1) but with a different node group

	beaconID := "default"
	nodeCount := 3
	mb := newMessageBus()

	// first we create some nodes
	nodes := make([]*stubbedDKGProcess, nodeCount)
	identities := make([]*dkg.Participant, nodeCount)
	for i := 0; i < nodeCount; i++ {
		stub, err := newStubbedDKGProcess(t, fmt.Sprintf("a:888%d", i), mb, beaconID)
		require.NoError(t, err)
		identity, err := util.PublicKeyAsParticipant(stub.key.Public)
		require.NoError(t, err)

		nodes[i] = stub
		identities[i] = identity
	}

	// then we run the DKG
	leader, err := nodes[0].RunnerFor(beaconID)
	require.NoError(t, err)
	err = leader.StartNetwork(2, 1, crypto.DefaultSchemeID, 1*time.Minute, 1, identities)
	require.NoError(t, err)
	for _, node := range nodes[1:] {
		r, err := node.RunnerFor(beaconID)
		require.NoError(t, err)
		require.NoError(t, r.JoinDKG())
	}

	require.NoError(t, leader.Abort())

	// we replace the last node with a new one
	stub, err := newStubbedDKGProcess(t, fmt.Sprintf("a:888%d", nodeCount+1), mb, beaconID)
	require.NoError(t, err)
	identity, err := util.PublicKeyAsParticipant(stub.key.Public)
	require.NoError(t, err)

	nodes[nodeCount-1] = stub
	identities[nodeCount-1] = identity

	// then we run the DKG again
	require.NoError(t, err)
	err = leader.StartNetwork(2, 1, crypto.DefaultSchemeID, 1*time.Minute, 1, identities)
	require.NoError(t, err)
	for _, node := range nodes[1:] {
		r, err := node.RunnerFor(beaconID)
		require.NoError(t, err)
		require.NoError(t, r.JoinDKG())
	}

	require.NoError(t, leader.StartExecution())
	require.NoError(t, leader.WaitForDKG(log.DefaultLogger(), 1, 60))
}

// stubbedBeacon wraps the keypair used by the `BeaconProcess`
type stubbedBeacon struct {
	kp *key.Pair
}

func (s stubbedBeacon) KeypairFor(_ string) (*key.Pair, error) {
	return s.kp, nil
}

// messageBus manages messaging between DKG processes without having to actually use gRPC
type messageBus struct {
	lock      sync.Mutex
	listeners map[string]dkg.DKGPublicClient
}

func newMessageBus() *messageBus {
	return &messageBus{
		listeners: make(map[string]dkg.DKGPublicClient),
	}
}

func (m *messageBus) Add(address string, process dkg.DKGPublicClient) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.listeners[address] = process
}

func (m *messageBus) Packet(
	_ context.Context,
	p net.Peer,
	packet *dkg.GossipPacket,
	_ ...grpc.CallOption,
) (*dkg.EmptyDKGResponse, error) {
	m.lock.Lock()
	listener := m.listeners[p.Address()]
	m.lock.Unlock()
	if listener == nil {
		return nil, errors.New("no such address")
	}
	return listener.Packet(context.Background(), packet)
}

func (m *messageBus) BroadcastDKG(
	_ context.Context,
	p net.Peer,
	in *dkg.DKGPacket,
	_ ...grpc.CallOption,
) (*dkg.EmptyDKGResponse, error) {
	m.lock.Lock()
	listener := m.listeners[p.Address()]
	m.lock.Unlock()
	if listener == nil {
		return nil, errors.New("no such address")
	}
	return listener.BroadcastDKG(context.Background(), in)
}

// stubbedDKGProcess simulates errors and delegates to a real DKG process when not in an error state
// it has pairwise pointers with the TestRunner to simplify usage - naughty, naughty
type stubbedDKGProcess struct {
	lock     sync.Mutex
	delegate *Process
	runners  []*TestRunner
	key      *key.Pair
	broken   bool
}

func newStubbedDKGProcess(t *testing.T, name string, bus *messageBus, beaconIDs ...string) (*stubbedDKGProcess, error) {
	dir := t.TempDir()
	store, err := NewDKGStore(dir)
	if err != nil {
		return nil, err
	}
	kp, err := key.NewKeyPair(name, crypto.NewPedersenBLSChained())
	if err != nil {
		return nil, err
	}

	out := util.NewFanOutChan[SharingOutput]()
	conf := Config{
		Timeout:              1 * time.Minute,
		TimeBetweenDKGPhases: 5 * time.Second,
		KickoffGracePeriod:   2 * time.Second,
		SkipKeyVerification:  false,
	}
	delegate := NewDKGProcess(store, stubbedBeacon{kp: kp}, out, bus, nil, conf, log.DefaultLogger().Named(name))
	wrapper := &stubbedDKGProcess{
		delegate: delegate,
		key:      kp,
		broken:   false,
	}

	runners := make([]*TestRunner, len(beaconIDs))
	for i, b := range beaconIDs {
		runner := TestRunner{
			Client:   wrapper,
			BeaconID: b,
			Clock:    clock.NewRealClock(),
		}

		runners[i] = &runner
	}
	wrapper.runners = runners
	bus.Add(name, wrapper)
	return wrapper, nil
}

func (p *stubbedDKGProcess) RunnerFor(beaconID string) (*TestRunner, error) {
	for _, r := range p.runners {
		if r.BeaconID == beaconID {
			return r, nil
		}
	}

	return nil, fmt.Errorf("no test runner found for beaconID %s", beaconID)
}

func (p *stubbedDKGProcess) Break() {
	p.lock.Lock()
	p.broken = true
	p.lock.Unlock()
}

func (p *stubbedDKGProcess) Fix() {
	p.lock.Lock()
	p.broken = false
	p.lock.Unlock()
}

func (p *stubbedDKGProcess) DKGStatus(
	ctx context.Context,
	request *dkg.DKGStatusRequest,
	_ ...grpc.CallOption,
) (*dkg.DKGStatusResponse, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.delegate.DKGStatus(ctx, request)
}

func (p *stubbedDKGProcess) Command(ctx context.Context, command *dkg.DKGCommand, _ ...grpc.CallOption) (*dkg.EmptyDKGResponse, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.delegate.Command(ctx, command)
}

func (p *stubbedDKGProcess) Packet(ctx context.Context, packet *dkg.GossipPacket, _ ...grpc.CallOption) (*dkg.EmptyDKGResponse, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.broken {
		return nil, errors.New("boom")
	}

	return p.delegate.Packet(ctx, packet)
}

func (p *stubbedDKGProcess) Migrate(beaconID string, group *key.Group, share *key.Share) error {
	return p.delegate.Migrate(beaconID, group, share)
}

func (p *stubbedDKGProcess) BroadcastDKG(
	ctx context.Context,
	packet *dkg.DKGPacket,
	_ ...grpc.CallOption,
) (*dkg.EmptyDKGResponse, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.broken {
		return nil, errors.New("boom")
	}

	return p.delegate.BroadcastDKG(ctx, packet)
}

func (p *stubbedDKGProcess) Close() {
	// no-op
}

func TestDKGWithCustomEntropySource(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	// Create a temporary script file that outputs random bytes
	scriptContent := `#!/bin/sh
echo "customrandombytes"
`
	tmpFile, err := os.CreateTemp("", "test-entropy-*.sh")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(scriptContent)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Make the script executable
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		t.Fatalf("Failed to make script executable: %v", err)
	}

	// Set the environment variable to use our custom entropy source
	originalValue := os.Getenv("DRAND_ENTROPY_SOURCE")
	err = os.Setenv("DRAND_ENTROPY_SOURCE", tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to set entropy source environment variable: %v", err)
	}
	// Restore the original value when the test completes
	defer func() {
		if originalValue == "" {
			os.Unsetenv("DRAND_ENTROPY_SOURCE")
		} else {
			os.Setenv("DRAND_ENTROPY_SOURCE", originalValue)
		}
	}()

	beaconID := "default"
	nodeCount := 3
	mb := newMessageBus()

	nodes := make([]*stubbedDKGProcess, nodeCount)
	identities := make([]*dkg.Participant, nodeCount)
	for i := 0; i < nodeCount; i++ {
		stub, err := newStubbedDKGProcess(t, fmt.Sprintf("a:888%d", i), mb, beaconID)
		require.NoError(t, err)
		identity, err := util.PublicKeyAsParticipant(stub.key.Public)
		require.NoError(t, err)

		nodes[i] = stub
		identities[i] = identity
	}

	// the leader kicks off a DKG
	leaderNode := nodes[0]
	leader, err := leaderNode.RunnerFor(beaconID)
	require.NoError(t, err)
	err = leader.StartNetwork(2, 1, crypto.DefaultSchemeID, 2*time.Minute, 1, identities)
	require.NoError(t, err)

	// the nodes all join
	for _, n := range nodes[1:] {
		r, err := n.RunnerFor(beaconID)
		require.NoError(t, err)
		err = r.JoinDKG()
		require.NoError(t, err)
	}

	// the leader kicks off execution
	err = leader.StartExecution()
	require.NoError(t, err)

	// wait for the DKG to complete
	err = leader.WaitForDKG(log.DefaultLogger(), 1, 30)
	require.NoError(t, err)

	// verify the DKG completed successfully
	successfulStatus, err := leaderNode.DKGStatus(context.Background(), &dkg.DKGStatusRequest{BeaconID: beaconID})
	require.NoError(t, err)
	require.Equal(t, Complete.String(), Status(successfulStatus.Current.State).String())
	require.Equal(t, Complete.String(), Status(successfulStatus.Complete.State).String())

	// The test succeeds if we get here, because it means the DKG process completed
	// successfully using our custom entropy source
}
