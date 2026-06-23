package interop_test

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common"
	chaininfo "github.com/drand/drand/v2/common/chain"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/common/testlogger"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/chain/boltdb"
	"github.com/drand/drand/v2/internal/interop"
)

// The fixtures below are real beacons fetched from the drand League of Entropy
// mainnet relays (https://api.drand.sh). They were produced with the legacy
// github.com/drand/kyber stack and must keep verifying under go.dedis.ch/kyber/v4.
//
//   - the "default" chain uses the chained pedersen-bls-chained scheme
//     (public key on G1, signatures on G2)
//   - the "quicknet" chain uses bls-unchained-g1-rfc9380
//     (public key on G2, signatures on G1)
//
// Both use the standard RFC9380 domain separation tags and must therefore verify
// under every BLS12-381 backend kyber v4 ships.

const defaultInfoJSON = `{"public_key":"868f005eb8e6e4ca0a47c8a77ceaa5309a47978a7c71bc5cce96366b5d7a569937c529eeda66c7293784a9402801af31","period":30,"genesis_time":1595431050,"hash":"8990e7a9aaed2ffed73dbd7092123d6f289930540d7651336225dc172e51b2ce","groupHash":"176f93498eac9ca337150b46d21dd58673ea4e3581185f869672e59fa4cb390a","schemeID":"pedersen-bls-chained","metadata":{"beaconID":"default"}}`

var defaultBeacons = []string{
	`{"round":1,"randomness":"101297f1ca7dc44ef6088d94ad5fb7ba03455dc33d53ddb412bbc4564ed986ec","signature":"8d61d9100567de44682506aea1a7a6fa6e5491cd27a0a0ed349ef6910ac5ac20ff7bc3e09d7c046566c9f7f3c6f3b10104990e7cb424998203d8f7de586fb7fa5f60045417a432684f85093b06ca91c769f0e7ca19268375e659c2a2352b4655","previous_signature":"176f93498eac9ca337150b46d21dd58673ea4e3581185f869672e59fa4cb390a"}`,
	`{"round":2,"randomness":"e8fee7dac6eb2b89df97d631cfccedbada7d5d05495bb546eef462e4145fdf8f","signature":"aa18facd2d51b616511d542de6f9af8a3b920121401dad1434ed1db4a565f10e04fad8d9b2b4e3e0094364374caafe9b10478bf75650124831509c638b5a36a7a232ec70289f8751a2adb47fc32eb70b57dc81c39d48cbcac9fec46cdfc31663","previous_signature":"8d61d9100567de44682506aea1a7a6fa6e5491cd27a0a0ed349ef6910ac5ac20ff7bc3e09d7c046566c9f7f3c6f3b10104990e7cb424998203d8f7de586fb7fa5f60045417a432684f85093b06ca91c769f0e7ca19268375e659c2a2352b4655"}`,
	`{"round":3,"randomness":"5e0c316703de0d11cc63439a26a5082ce966f4f1e4068cd64b35fee906a0f84b","signature":"a7b0877eaea7a0222f4c39a2c03434c34f5fe3ea47c533d24b88e5c3053b84775ccb78e984addcb55173f40428513f280cc6e0fccc3c89bb1625c7c0b477deb6faae43fc6ec036f09233bf38da16586b3042dd01a7e9ed97c8bafa343cc6071e","previous_signature":"aa18facd2d51b616511d542de6f9af8a3b920121401dad1434ed1db4a565f10e04fad8d9b2b4e3e0094364374caafe9b10478bf75650124831509c638b5a36a7a232ec70289f8751a2adb47fc32eb70b57dc81c39d48cbcac9fec46cdfc31663"}`,
}

const quicknetInfoJSON = `{"public_key":"83cf0f2896adee7eb8b5f01fcad3912212c437e0073e911fb90022d3e760183c8c4b450b6a0a6c3ac6a5776a2d1064510d1fec758c921cc22b0e17e63aaf4bcb5ed66304de9cf809bd274ca73bab4af5a6e9c76a4bc09e76eae8991ef5ece45a","period":3,"genesis_time":1692803367,"hash":"52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971","groupHash":"f477d5c89f21a17c863a7f937c6a6d15859414d2be09cd448d4279af331c5d3e","schemeID":"bls-unchained-g1-rfc9380","metadata":{"beaconID":"quicknet"}}`

var quicknetBeacons = []string{
	`{"round":1,"randomness":"1466a6cd24e327188770752f6134001c64d6efcc590ccc26b721611ad96f165a","signature":"b55e7cb2d5c613ee0b2e28d6750aabbb78c39dcc96bd9d38c2c2e12198df95571de8e8e402a0cc48871c7089a2b3af4b"}`,
	`{"round":2,"randomness":"5782d6987841c654515a0e72b2d1ebb4e741234042c37cb19608ae50d93fb60c","signature":"b6b6a585449b66eb12e875b64fcbab3799861a00e4dbf092d99e969a5eac57dd3f798acf61e705fe4f093db926626807"}`,
	`{"round":3,"randomness":"7ef4621ace1c6da4eb2eee7cd901f81385bca5b189771ec0f08d0d2566dd1a21","signature":"b3fab6df720b68cc47175f2c777e86d84187caab5770906f515ff1099cb01e4deaa027075d860823e49477b93c72bd64"}`,
}

func parseInfo(t *testing.T, raw string) *chaininfo.Info {
	t.Helper()
	info := new(chaininfo.Info)
	require.NoError(t, json.Unmarshal([]byte(raw), info))
	return info
}

func parseBeacons(t *testing.T, raws []string) []*common.Beacon {
	t.Helper()
	beacons := make([]*common.Beacon, 0, len(raws))
	for _, raw := range raws {
		b := new(common.Beacon)
		require.NoError(t, json.Unmarshal([]byte(raw), b))
		beacons = append(beacons, b)
	}
	return beacons
}

// writeBoltDB writes the given beacons into a fresh (untrimmed) boltdb store and
// returns the folder containing the resulting drand.db file.
func writeBoltDB(t *testing.T, l log.Logger, beacons []*common.Beacon) string {
	t.Helper()
	folder := t.TempDir()
	ctx := boltdb.IsATest(context.Background())
	store, err := boltdb.NewBoltStore(ctx, l, folder)
	require.NoError(t, err)
	for _, b := range beacons {
		require.NoError(t, store.Put(ctx, b))
	}
	require.NoError(t, store.Close())
	return folder
}

func TestVerifyLiveChains(t *testing.T) {
	cases := []struct {
		name     string
		info     string
		beacons  []string
		expected int
	}{
		{"chained-default", defaultInfoJSON, defaultBeacons, len(defaultBeacons)},
		{"g1-quicknet", quicknetInfoJSON, quicknetBeacons, len(quicknetBeacons)},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			l := testlogger.New(t)
			info := parseInfo(t, tc.info)
			folder := writeBoltDB(t, l, parseBeacons(t, tc.beacons))

			// Open the boltdb file from disk and verify all beacons across every backend.
			results, err := interop.VerifyBoltDB(context.Background(), l, folder, info, interop.BLS12381Backends)
			require.NoError(t, err)
			require.Len(t, results, len(interop.BLS12381Backends))

			for _, backend := range interop.BLS12381Backends {
				res := results[backend]
				require.NotNil(t, res, "missing result for backend %s", backend)
				require.Falsef(t, res.Unsupported, "backend %s unexpectedly unsupported: %s", backend, res.UnsupportedReason)
				require.Zerof(t, res.Failed, "backend %s failed to verify round %d", backend, res.FirstFailedRound)
				require.Equalf(t, tc.expected, res.Verified, "backend %s verified count mismatch", backend)
				t.Logf("backend %-6s verified %d/%d beacons", backend, res.Verified, tc.expected)
			}
		})
	}
}

// TestTamperedSignatureFails ensures the verifier actually rejects a bad signature
// rather than blindly accepting everything.
func TestTamperedSignatureFails(t *testing.T) {
	l := testlogger.New(t)
	info := parseInfo(t, quicknetInfoJSON)
	beacons := parseBeacons(t, quicknetBeacons)

	// flip a byte in the last beacon's signature
	tampered := beacons[len(beacons)-1].Signature
	tampered[0] ^= 0xff

	folder := writeBoltDB(t, l, beacons)
	results, err := interop.VerifyBoltDB(context.Background(), l, folder, info, interop.BLS12381Backends)
	require.NoError(t, err)

	for _, backend := range interop.BLS12381Backends {
		res := results[backend]
		require.Equalf(t, 1, res.Failed, "backend %s should have detected the tampered beacon", backend)
		require.Equal(t, uint64(3), res.FirstFailedRound)
	}
}

// TestSwappedSchemeBackendSupport documents that the deprecated bls-unchained-on-g1
// scheme, which uses a non-standard hash-to-curve domain, can only be reproduced by
// the kilic backend; circl and gnark are reported as unsupported.
func TestSwappedSchemeBackendSupport(t *testing.T) {
	sch, err := crypto.SchemeFromName(crypto.ShortSigSchemeID)
	require.NoError(t, err)
	// dummy public key bytes of the right length so kilic gets past unmarshalling
	rawPub, err := sch.KeyGroup.Point().Base().MarshalBinary()
	require.NoError(t, err)

	_, err = interop.NewVerifier(crypto.ShortSigSchemeID, "kilic", rawPub)
	require.NoError(t, err, "kilic must support the swapped scheme")

	for _, backend := range []string{"circl", "gnark"} {
		_, err := interop.NewVerifier(crypto.ShortSigSchemeID, backend, rawPub)
		require.ErrorIs(t, err, interop.ErrUnsupportedBackend, "%s must report the swapped scheme as unsupported", backend)
	}
}

// TestVerifyExternalDB lets an operator point the interop verifier at a real boltdb
// dump. Set DRAND_INTEROP_DB to the folder containing drand.db and
// DRAND_INTEROP_CHAININFO to a chain-info JSON file (as served on /info). The test
// is skipped when these are not set.
func TestVerifyExternalDB(t *testing.T) {
	dbFolder := os.Getenv("DRAND_INTEROP_DB")
	infoPath := os.Getenv("DRAND_INTEROP_CHAININFO")
	if dbFolder == "" || infoPath == "" {
		t.Skip("set DRAND_INTEROP_DB and DRAND_INTEROP_CHAININFO to verify an external boltdb dump")
	}

	rawInfo, err := os.ReadFile(infoPath)
	require.NoError(t, err)
	info := new(chaininfo.Info)
	require.NoError(t, json.Unmarshal(rawInfo, info))

	l := testlogger.New(t)
	t.Logf("verifying boltdb %q (scheme %q, chainhash %s)", path.Join(dbFolder, boltdb.BoltFileName), info.Scheme, info.HashString())

	results, err := interop.VerifyBoltDB(context.Background(), l, dbFolder, info, interop.BLS12381Backends)
	require.NoError(t, err)

	for _, backend := range interop.BLS12381Backends {
		res := results[backend]
		if res.Unsupported {
			t.Logf("backend %-6s unsupported: %s", backend, res.UnsupportedReason)
			continue
		}
		t.Logf("backend %-6s verified %d, failed %d (first failed round %d)", backend, res.Verified, res.Failed, res.FirstFailedRound)
		require.Zerof(t, res.Failed, "backend %s failed to verify round %d", backend, res.FirstFailedRound)
		require.Positivef(t, res.Verified, "backend %s verified nothing", backend)
	}
}
