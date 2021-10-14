package client_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/drand/drand/client"
	"github.com/drand/drand/client/test/result/mock"
	"github.com/drand/drand/common/scheme"
)

func mockClientWithVerifiableResults(n int) (client.Client, []mock.Result, error) {
	sch := scheme.GetSchemeFromEnv()

	info, results := mock.VerifiableResults(n, sch)
	mc := client.MockClient{Results: results, StrictRounds: true}

	var c client.Client
	var err error

	c, err = client.Wrap(
		[]client.Client{client.MockClientWithInfo(info), &mc},
		client.WithChainInfo(info),
		client.WithVerifiedResult(&results[0]),
		client.WithFullChainVerification(),
		client.WithSchemeID(sch.ID),
	)

	if err != nil {
		return nil, nil, err
	}
	return c, results, nil
}

func TestVerify(t *testing.T) {
	VerifyFuncTest(t, 3, 1)
}

func TestVerifyWithOldVerifiedResult(t *testing.T) {
	VerifyFuncTest(t, 5, 4)
}

func VerifyFuncTest(t *testing.T, clients, upTo int) {
	c, results, err := mockClientWithVerifiableResults(clients)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Get(context.Background(), results[upTo].Round())
	if err != nil {
		t.Fatal(err)
	}
	if res.Round() != results[upTo].Round() {
		t.Fatal("expected to get result.", results[upTo].Round(), res.Round(), fmt.Sprintf("%v", c))
	}
}
