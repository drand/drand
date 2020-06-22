package client

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/drand/drand/chain"
)

func TestEmptyClient(t *testing.T) {
	chainInfo := fakeChainInfo()
	c := EmptyClientWithInfo(chainInfo)

	// should be able to retrieve Info
	i, err := c.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if i != chainInfo {
		t.Fatal("unexpected chain info", i)
	}

	// should be able to retrieve RoundAt
	now := time.Now()
	rnd := c.RoundAt(now)
	if rnd != chain.CurrentRound(now.Unix(), chainInfo.Period, chainInfo.GenesisTime) {
		t.Fatal("unexpected RoundAt return value", rnd)
	}

	// should be fmt.Stringer
	sc, ok := c.(fmt.Stringer)
	if !ok {
		t.Fatal("expected Stringer interface")
	}
	if sc.String() != emptyClientStringerValue {
		t.Fatal("unexpected string value")
	}

	// but Get does not work
	_, err = c.Get(context.Background(), 0)
	if err == nil {
		t.Fatal("expected an error")
	}
	if err.Error() != "not supported" {
		t.Fatal("unexpected error from Get", err)
	}

	// and Watch returns an empty closed channel
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	ch := c.Watch(ctx)
	rs := []Result{}
	for r := range ch {
		rs = append(rs, r)
	}

	if len(rs) > 0 {
		t.Fatal("unexpected results in watch channel", rs)
	}

	if err := c.Close(); err != nil {
		t.Fatal("unexpected error closing client", err)
	}
}
