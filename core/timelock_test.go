package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/encrypt/timelock"
	"github.com/stretchr/testify/require"
)

func TestTimelock(t *testing.T) {
	n := 4
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	dt := NewDrandTest2(t, n, thr, p)
	defer dt.Cleanup()
	group := dt.RunDKG()
	time.Sleep(getSleepDuration())
	root := dt.nodes[0].drand
	rootID := root.priv.Public

	dt.MoveToTime(group.GenesisTime)
	// do a few periods
	for i := 0; i < 3; i++ {
		dt.MoveTime(group.Period)
	}

	cm := root.opts.certmanager
	client := net.NewGrpcClientFromCertManager(cm)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// get round
	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(t, err)

	// encrypt a message
	msg := []byte("Open this in year 2100")
	toRound := resp.Round + 3
	id := chain.MessageV2(toRound)
	sig, err := timelock.Encrypt(key.Pairing, key.KeyGroup.Point().Base(), group.PublicKey.Key(), id, msg)
	require.NoError(t, err)

	round := resp.Round + 1
	for round <= toRound {
		dt.MoveTime(group.Period)
		req := new(drand.PublicRandRequest)
		req.Round = round
		resp, err := client.PublicRand(ctx, rootID, req)
		require.NoError(t, err)
		require.Equal(t, round, resp.Round)
		private := key.SigGroup.Point()
		err = private.UnmarshalBinary(resp.SignatureV2)
		require.NoError(t, err)
		msg2, err := timelock.Decrypt(key.Pairing, private, sig)
		// XXX Currently there is no MAC to ensure the message is the right one
		// so there should never be an error when trying to decrypt (it happens
		// only if length isn't right size etc) - we could add that in kyber
		// according to necessity but drand doesn't need to deal with that.
		require.NoError(t, err)
		if round == toRound {
			require.Equal(t, msg, msg2)
			fmt.Println(" MESSAGE DECRYPTED ")
		} else {
			require.NotEqual(t, msg, msg2)
		}
		round++
	}
}
