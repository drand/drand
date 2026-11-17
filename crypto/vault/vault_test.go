package vault

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/chain"
	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/common/testlogger"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/test"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/share/dkg"
	"github.com/drand/kyber/util/random"
)

// distKey generates a coherent threshold key: a shared public polynomial and the
// n private shares consistent with it, mirroring the output of a real DKG.
func distKey(sch *crypto.Scheme, n, thr int) (commits []kyber.Point, shares []*share.PriShare) {
	secret := sch.KeyGroup.Scalar().Pick(random.New())
	priPoly := share.NewPriPoly(sch.KeyGroup, thr, secret, random.New())
	pubPoly := priPoly.Commit(sch.KeyGroup.Point().Base())
	_, commits = pubPoly.Info()
	return commits, priPoly.Shares(n)
}

// newGroupWithShares builds a key.Group whose distributed public key matches the
// returned per-node shares, so that partial signatures actually recover and verify.
// It takes testing.TB so both tests and benchmarks can use it.
func newGroupWithShares(_ testing.TB, sch *crypto.Scheme, n, thr int) (*key.Group, []*key.Share) {
	privs := test.GenerateIDs(n)
	commits, priShares := distKey(sch, n, thr)

	group := key.LoadGroup(
		test.ListFromPrivates(privs),
		1, // genesis time
		&key.DistPublic{Coefficients: commits},
		30*time.Second,
		0, // transition time
		sch,
		"vault-test",
	)
	group.Threshold = thr

	shares := make([]*key.Share, n)
	for i := 0; i < n; i++ {
		shares[i] = &key.Share{
			DistKeyShare: dkg.DistKeyShare{Share: priShares[i], Commits: commits},
			Scheme:       sch,
		}
	}
	return group, shares
}

// TestVaultPartialRoundTrip is the core safety net: it exercises the full
// threshold-signing hot path (per-node partial sign -> aggregate -> verify) for
// every scheme drand supports. This is exactly the path that the kyber upgrade
// changes under the hood, so it must keep working byte-for-byte.
func TestVaultPartialRoundTrip(t *testing.T) {
	const n, thr = 5, 3

	for _, name := range crypto.ListSchemes() {
		name := name
		t.Run(name, func(t *testing.T) {
			sch, err := crypto.SchemeFromName(name)
			require.NoError(t, err)

			group, shares := newGroupWithShares(t, sch, n, thr)
			pubPoly := group.PublicKey.PubPoly(sch)

			b := &common.Beacon{Round: 42, PreviousSig: []byte("previous")}
			msg := sch.DigestBeacon(b)

			partials := make([][]byte, 0, n)
			for i := 0; i < n; i++ {
				v := NewVault(testlogger.New(t), group, shares[i], sch)

				partial, err := v.SignPartial(msg)
				require.NoError(t, err)

				// every partial must verify against the shared public polynomial
				require.NoError(t, sch.ThresholdScheme.VerifyPartial(pubPoly, msg, partial))
				partials = append(partials, partial)
			}

			// only a threshold of partials is required to recover the full signature
			sig, err := sch.ThresholdScheme.Recover(pubPoly, msg, partials[:thr], thr, n)
			require.NoError(t, err)

			b.Signature = sig
			require.NoError(t, sch.VerifyBeacon(b, group.PublicKey.Key()))
		})
	}
}

func TestVaultIndexMatchesShare(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	group, shares := newGroupWithShares(t, sch, 4, 3)

	for i, sh := range shares {
		v := NewVault(testlogger.New(t), group, sh, sch)
		require.Equal(t, sh.Share.I, v.Index())
		require.Equal(t, i, v.Index(), "share index should match node position")
	}
}

func TestVaultGetters(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	group, shares := newGroupWithShares(t, sch, 4, 3)

	v := NewVault(testlogger.New(t), group, shares[0], sch)

	require.Same(t, group, v.GetGroup())
	require.NotNil(t, v.GetPub())
	require.NotNil(t, v.GetInfo())
	// the chain info exposed by the vault must match the one derived from the group
	require.True(t, v.GetInfo().Equal(chain.NewChainInfo(group)))
}

func TestVaultSetInfoKeepsChainInfoConstant(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	group, shares := newGroupWithShares(t, sch, 4, 3)
	v := NewVault(testlogger.New(t), group, shares[0], sch)

	infoBefore := v.GetInfo()
	pubBefore := v.GetPub()

	// reshare-like update: new group + new share for this node
	newGroup, newShares := newGroupWithShares(t, sch, 4, 3)
	newGroup.GenesisTime = group.GenesisTime // chain info is invariant across resharing
	newGroup.GenesisSeed = group.GetGenesisSeed()
	v.SetInfo(newGroup, newShares[1])

	require.Same(t, newGroup, v.GetGroup())
	require.Equal(t, newShares[1].Share.I, v.Index())
	require.NotSame(t, pubBefore, v.GetPub(), "public polynomial should be refreshed")
	// chain info is documented as constant across SetInfo
	require.True(t, infoBefore.Equal(v.GetInfo()))
}

// TestVaultConcurrentAccess asserts the documented thread-safety of Vault: readers
// (SignPartial/Index/GetPub) must be safe to run concurrently with a writer
// (SetInfo). Run with -race to catch data races; it also guards against lock
// regressions that could deadlock the signing path.
func TestVaultConcurrentAccess(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	group, shares := newGroupWithShares(t, sch, 4, 3)
	v := NewVault(testlogger.New(t), group, shares[0], sch)
	msg := []byte("concurrent message")

	done := make(chan struct{})
	var wg sync.WaitGroup

	// readers
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					_, _ = v.SignPartial(msg)
					_ = v.Index()
					_ = v.GetPub()
					_ = v.GetInfo()
				}
			}
		}()
	}

	// writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			select {
			case <-done:
				return
			default:
				ng, ns := newGroupWithShares(t, sch, 4, 3)
				v.SetInfo(ng, ns[0])
			}
		}
	}()

	time.Sleep(200 * time.Millisecond)
	close(done)
	wg.Wait()
}
