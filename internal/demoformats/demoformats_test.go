package demoformats

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/common/testlogger"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/chain"
	"github.com/drand/drand/v2/internal/chain/boltdb"
	chainerrors "github.com/drand/drand/v2/internal/chain/errors"
	"github.com/drand/drand/v2/internal/dkg"
)

const beaconID = "default"

// nodeDir copies testdata/node into a fresh temp dir. The boltdb-backed stores
// open their files read-write and create folders, so we work on a copy to keep
// the committed fixtures pristine.
func nodeDir(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	src := filepath.Join("testdata", "node")
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
	require.NoError(t, err)
	return dst
}

func fileStore(t *testing.T, dir string) key.Store {
	t.Helper()
	return key.NewFileStore(filepath.Join(dir, common.MultiBeaconFolder), beaconID)
}

// TestGroupFileParses loads the drand_group.toml a node persists after a DKG.
func TestGroupFileParses(t *testing.T) {
	g, err := fileStore(t, nodeDir(t)).LoadGroup()
	require.NoError(t, err)

	require.Equal(t, beaconID, g.ID)
	require.Equal(t, crypto.DefaultSchemeID, g.Scheme.Name)
	require.Equal(t, 4, g.Threshold)
	require.Len(t, g.Nodes, 6)
	require.Equal(t, time.Second, g.Period)
	require.NotNil(t, g.PublicKey)
	require.NotEmpty(t, g.PublicKey.Coefficients)
	require.NotEmpty(t, g.GetGenesisSeed())

	// every node identity in the group must carry a valid self-signature
	for _, n := range g.Nodes {
		require.NoError(t, n.Identity.ValidSignature(), "node %d self-signature", n.Index)
	}
}

// TestKeyPairParses loads drand_id.private / drand_id.public.
func TestKeyPairParses(t *testing.T) {
	kp, err := fileStore(t, nodeDir(t)).LoadKeyPair()
	require.NoError(t, err)

	require.NotNil(t, kp.Key)
	require.NotNil(t, kp.Public)
	require.NotEmpty(t, kp.Public.Addr)
	require.NoError(t, kp.Public.ValidSignature())
	require.Equal(t, crypto.DefaultSchemeID, kp.Scheme().Name)
}

// TestShareParses loads the distributed key share dist_key.private.
func TestShareParses(t *testing.T) {
	dir := nodeDir(t)
	sh, err := fileStore(t, dir).LoadShare()
	require.NoError(t, err)

	require.NotNil(t, sh.Scheme)
	require.NotNil(t, sh.PrivateShare())
	require.NotEmpty(t, sh.Commits)

	// the share's public commitments must match the group's distributed public key
	g, err := fileStore(t, dir).LoadGroup()
	require.NoError(t, err)
	require.Len(t, sh.Commits, len(g.PublicKey.Coefficients))
	require.True(t, sh.Public().Equal(g.PublicKey))
}

// TestBeaconDBParses opens the beacon boltdb, reads every stored beacon and
// verifies each signature against the group's distributed public key. The demo
// produces a trimmed store, which the loader must transparently handle.
func TestBeaconDBParses(t *testing.T) {
	dir := nodeDir(t)
	l := testlogger.New(t)

	g, err := fileStore(t, dir).LoadGroup()
	require.NoError(t, err)
	scheme := g.Scheme
	pub := g.PublicKey.Key()

	// chained schemes reconstruct the previous signature on read
	ctx := chain.SetPreviousRequiredOnContext(context.Background())
	dbFolder := filepath.Join(dir, common.MultiBeaconFolder, beaconID, "db")
	store, err := boltdb.NewBoltStore(ctx, l, dbFolder)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	length, err := store.Len(ctx)
	require.NoError(t, err)
	require.Positive(t, length, "beacon store should not be empty")

	last, err := store.Last(ctx)
	require.NoError(t, err)
	require.NotNil(t, last)
	require.Positive(t, last.Round)

	verified := 0
	err = store.Cursor(ctx, func(ctx context.Context, c chain.Cursor) error {
		b, err := c.First(ctx)
		for ; b != nil; b, err = c.Next(ctx) {
			if err != nil {
				return err
			}
			if b.Round == 0 {
				// genesis: no signature to verify
				continue
			}
			require.NoErrorf(t, scheme.VerifyBeacon(b, pub), "round %d failed to verify", b.Round)
			verified++
		}
		if err != nil && !errors.Is(err, chainerrors.ErrNoBeaconStored) {
			return err
		}
		return nil
	})
	require.NoError(t, err)
	require.Positive(t, verified, "expected at least one signed beacon to verify")
	t.Logf("verified %d beacons up to round %d", verified, last.Round)
}

// TestDKGDBParses opens the DKG state boltdb and decodes the staged and finished
// DKG states a node stores.
func TestDKGDBParses(t *testing.T) {
	dir := nodeDir(t)
	store, err := dkg.NewDKGStore(dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	finished, err := store.GetFinished(beaconID)
	require.NoError(t, err)
	require.NotNil(t, finished)
	require.Equal(t, beaconID, finished.BeaconID)
	require.Positive(t, finished.Epoch)
	require.Equal(t, uint32(4), finished.Threshold)
	require.NotNil(t, finished.FinalGroup, "a finished DKG must carry its final group")
	require.Len(t, finished.FinalGroup.Nodes, 6)

	// GetCurrent returns the latest staged state; it must decode too.
	current, err := store.GetCurrent(beaconID)
	require.NoError(t, err)
	require.NotNil(t, current)
	require.Equal(t, beaconID, current.BeaconID)
}
