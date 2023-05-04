package client_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/drand/drand/crypto"

	"github.com/stretchr/testify/require"

	client2 "github.com/drand/drand/client"
	clientMock "github.com/drand/drand/client/mock"
	"github.com/drand/drand/client/test/result/mock"
	"github.com/drand/drand/common/client"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/test/testlogger"
)

func mockClientWithVerifiableResults(ctx context.Context, t *testing.T, l log.Logger, n int) (client.Client, []mock.Result) {
	t.Helper()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	info, results := mock.VerifiableResults(n, sch)
	mc := clientMock.Client{Results: results, StrictRounds: true, OptionalInfo: info}

	var c client.Client

	c, err = client2.Wrap(
		ctx,
		l,
		[]client.Client{clientMock.ClientWithInfo(info), &mc},
		client2.WithChainInfo(info),
		client2.WithVerifiedResult(&results[0]),
		client2.WithFullChainVerification(),
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
	ctx := context.Background()
	l := testlogger.New(t)
	c, results := mockClientWithVerifiableResults(ctx, t, l, clients)

	res, err := c.Get(context.Background(), results[upTo].Round())
	require.NoError(t, err)

	if res.Round() != results[upTo].Round() {
		t.Fatal("expected to get result.", results[upTo].Round(), res.Round(), fmt.Sprintf("%v", c))
	}
}
