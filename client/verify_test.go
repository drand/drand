package client_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/client/test/result/mock"
	"github.com/stretchr/testify/require"
)

func mockClientWithVerifiableResults(n int) (client.Client, []mock.Result, error) {
	info, results := mock.VerifiableResults(n)
	mc := client.MockClient{Results: results, StrictRounds: true}
	c, err := client.Wrap(
		[]client.Client{client.MockClientWithInfo(info), &mc},
		client.WithChainInfo(info),
		client.WithVerifiedResult(&results[0]),
		client.WithFullChainVerification(),
		client.WithV2From(uint64(10000000)),
	)
	if err != nil {
		return nil, nil, err
	}
	return c, results, nil
}

func TestVerifyToV2(t *testing.T) {
	n := 10
	var v2from uint64 = 5
	info, results := mock.VerifiableResults(n)
	mc := client.MockClient{Results: results, StrictRounds: true}
	c, err := client.Wrap(
		[]client.Client{client.MockClientWithInfo(info), &mc},
		client.WithChainInfo(info),
		client.WithVerifiedResult(&results[0]),
		client.WithFullChainVerification(),
		client.WithV2From(v2from),
	)
	require.NoError(t, err)
	for i := results[0].Round(); i <= uint64(n); i++ {
		res, err := c.Get(context.Background(), uint64(i))
		require.NoError(t, err)
		require.Equal(t, res.Round(), uint64(i))
		if i >= v2from {
			require.NoError(t, chain.VerifyBeaconV2(info.PublicKey, &chain.Beacon{
				Round:       uint64(i),
				SignatureV2: res.SignatureV2(),
			}))
		}
	}

}

func TestVerifySimple(t *testing.T) {
	c, results, err := mockClientWithVerifiableResults(3)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Get(context.Background(), results[1].Round())
	if err != nil {
		t.Fatal(err)
	}
	if res.Round() != results[1].Round() {
		t.Fatal("expected to get result.", results[1].Round(), res.Round(), fmt.Sprintf("%v", c))
	}
}

func TestVerifyWithOldVerifiedResult(t *testing.T) {
	c, results, err := mockClientWithVerifiableResults(5)
	if err != nil {
		t.Fatal(err)
	}
	// should automatically load rounds 1, 2 and 3 to verify 4
	res, err := c.Get(context.Background(), results[4].Round())
	if err != nil {
		t.Fatal(err)
	}
	if res.Round() != results[4].Round() {
		t.Fatal("expected to get result.", results[4].Round(), res.Round(), fmt.Sprintf("%v", c))
	}
}
