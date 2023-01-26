package client_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/crypto"

	"github.com/drand/drand/client"
	"github.com/drand/drand/client/test/result/mock"
)

func mockClientWithVerifiableResults(t *testing.T, n int) (client.Client, []mock.Result) {
	t.Helper()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	info, results := mock.VerifiableResults(n, sch)
	mc := client.MockClient{Results: results, StrictRounds: true, OptionalInfo: info}

	var c client.Client

	c, err = client.Wrap(
		[]client.Client{client.MockClientWithInfo(info), &mc},
		client.WithChainInfo(info),
		client.WithVerifiedResult(&results[0]),
		client.WithFullChainVerification(),
	)
	require.NoError(t, err)

	return c, results
}

func TestVerify(t *testing.T) {
	VerifyFuncTest(t, 3, 1)
}

func TestVerifyWithOldVerifiedResult(t *testing.T) {
	VerifyFuncTest(t, 5, 4)
}

func VerifyFuncTest(t *testing.T, clients, upTo int) {
	c, results := mockClientWithVerifiableResults(t, clients)

	res, err := c.Get(context.Background(), results[upTo].Round())
	require.NoError(t, err)

	if res.Round() != results[upTo].Round() {
		t.Fatal("expected to get result.", results[upTo].Round(), res.Round(), fmt.Sprintf("%v", c))
	}
}
