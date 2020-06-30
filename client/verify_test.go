package client_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/drand/drand/client"
	"github.com/drand/drand/client/test/result/mock"
)

func TestVerify(t *testing.T) {
	info, results := mock.VerifiableResults(3)
	mc := client.MockClient{Results: results, StrictRounds: true}
	c, err := client.Wrap(
		[]client.Client{client.MockClientWithInfo(info), &mc},
		client.WithChainInfo(info),
		client.WithVerifiedResult(&results[0]),
		client.WithFullChainVerification(),
	)
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
