package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
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
	sig, err := Encrypt(group.PublicKey, toRound, msg)
	require.NoError(t, err)
	fmt.Printf("TIME CAPSULE : %x\n", sig.xor)

	round := resp.Round + 1
	for round <= toRound {
		dt.MoveTime(group.Period)
		req := new(drand.PublicRandRequest)
		req.Round = round
		resp, err := client.PublicRand(ctx, rootID, req)
		require.NoError(t, err)
		require.Equal(t, round, resp.Round)
		private := key.SigGroup.Point()
		err = private.UnmarshalBinary(resp.Signature)
		require.NoError(t, err)
		msg2 := Decrypt(private, sig)
		if round == toRound {
			require.Equal(t, msg, msg2)
			fmt.Println(" MESSAGE DECRYPTED ")
		} else {
			require.NotEqual(t, msg, msg2)
		}
		round += 1
	}
}
