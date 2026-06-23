package key

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	commonutils "github.com/drand/drand/v2/common"
	"go.dedis.ch/kyber/v4"
	"go.dedis.ch/kyber/v4/share"
	dkg "go.dedis.ch/kyber/v4/share/dkg/pedersen"
)

func TestKeysSaveLoad(t *testing.T) {
	n := 4
	ps, group := BatchIdentities(t, n)
	// we don't use the function from the test package here to avoid a circular dependency
	beaconID := commonutils.GetCanonicalBeaconID(os.Getenv("BEACON_ID"))

	tmp := path.Join(t.TempDir(), "drand-key")

	store := NewFileStore(tmp, beaconID).(*fileStore)
	require.Equal(t, tmp, store.baseFolder)

	// test loading saving private public key
	require.NoError(t, store.SaveKeyPair(ps[0]))
	loadedKey, err := store.LoadKeyPair()
	require.NoError(t, err)

	require.Equal(t, loadedKey.Key.String(), ps[0].Key.String())
	require.Equal(t, loadedKey.Public.Key.String(), ps[0].Public.Key.String())
	require.Equal(t, loadedKey.Public.Scheme.Name, ps[0].Public.Scheme.Name)
	require.Equal(t, loadedKey.Public.Address(), ps[0].Public.Address())

	_, err = os.Stat(store.privateKeyFile)
	require.NoError(t, err)
	_, err = os.Stat(store.publicKeyFile)
	require.NoError(t, err)

	// test group
	require.NoError(t, store.SaveGroup(group))
	loadedGroup, err := store.LoadGroup()
	require.NoError(t, err)
	require.Equal(t, group.Threshold, loadedGroup.Threshold)
	require.Equal(t, commonutils.GetCanonicalBeaconID(group.ID), loadedGroup.ID, "group id must round-trip (canonical default when empty)")
	require.Equal(t, group.Period, loadedGroup.Period)
	require.Equal(t, group.CatchupPeriod, loadedGroup.CatchupPeriod)
	require.Equal(t, group.GenesisTime, loadedGroup.GenesisTime)
	require.Equal(t, group.TransitionTime, loadedGroup.TransitionTime)
	require.Equal(t, group.Scheme.Name, loadedGroup.Scheme.Name, "scheme must round-trip")
	require.NotNil(t, group.PublicKey)
	require.NotNil(t, loadedGroup.PublicKey)
	require.True(t, group.PublicKey.Equal(loadedGroup.PublicKey), "distributed public key must round-trip")

	// Compare nodes by address only; order in loadedGroup.Nodes must not matter.
	// Use the saved group as source of truth so we assert full Node equality (index + identity).
	expectedByAddr := make(map[string]*Node, len(group.Nodes))
	for _, node := range group.Nodes {
		expectedByAddr[node.Addr] = node
	}
	loadedByAddr := make(map[string]*Node, len(loadedGroup.Nodes))
	for _, n := range loadedGroup.Nodes {
		_, dup := loadedByAddr[n.Addr]
		require.False(t, dup, "duplicate address in loaded group: %s", n.Addr)
		loadedByAddr[n.Addr] = n
	}
	require.Len(t, loadedGroup.Nodes, len(expectedByAddr))
	require.Len(t, loadedByAddr, len(expectedByAddr), "same node count and no duplicate addresses")
	for addr, exp := range expectedByAddr {
		ln, ok := loadedByAddr[addr]
		require.True(t, ok, "missing node for address %s", addr)
		// Explicit checks (same contract as Node.Equal: index + identity Addr + key)
		require.Equal(t, addr, ln.Addr)
		require.Equal(t, exp.Index, ln.Index)
		require.True(t, exp.Key.Equal(ln.Key), "public key mismatch for %s", addr)
	}

	// test share / dist key
	testShare := &Share{
		DistKeyShare: dkg.DistKeyShare{
			Commits: []kyber.Point{ps[0].Public.Key, ps[1].Public.Key},
			Share:   &share.PriShare{V: ps[0].Key, I: 0},
		},
		Scheme: group.Scheme,
	}
	require.NoError(t, store.SaveShare(testShare))
	loadedShare, err := store.LoadShare()

	require.NoError(t, err)
	require.Equal(t, testShare.Scheme.Name, loadedShare.Scheme.Name)
	require.Equal(t, testShare.Share.V, loadedShare.Share.V)
	require.Equal(t, testShare.Share.I, loadedShare.Share.I)
	require.Len(t, loadedShare.Commits, len(testShare.Commits))
	for i := range testShare.Commits {
		require.True(t, testShare.Commits[i].Equal(loadedShare.Commits[i]), "commit %d", i)
	}
}

func TestTwoStores(t *testing.T) {
	// we don't use the function from the test package here to avoid a circular dependency
	beaconID := commonutils.GetCanonicalBeaconID(os.Getenv("BEACON_ID"))

	tmp := path.Join(t.TempDir(), "drand-key-2")

	store1 := NewFileStore(tmp, beaconID).(*fileStore)
	require.Equal(t, tmp, store1.baseFolder)
	store2 := NewFileStore(tmp, beaconID+"2").(*fileStore)
	require.Equal(t, tmp, store2.baseFolder)

	stores, err := NewFileStores(tmp)
	require.NoError(t, err)
	require.Contains(t, stores, store1.beaconID)
	require.Contains(t, stores, store2.beaconID)
}
