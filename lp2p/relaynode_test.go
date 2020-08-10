package lp2p

import (
	"context"
	"encoding/hex"
	"errors"
	"io/ioutil"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/client/test/result/mock"
	"github.com/drand/drand/log"
	"github.com/drand/drand/test"
)

type mockClient struct {
	chainInfo *chain.Info
	watchF    func(context.Context) <-chan client.Result
}

func (c *mockClient) Get(ctx context.Context, round uint64) (client.Result, error) {
	return nil, errors.New("unsupported")
}

func (c *mockClient) Watch(ctx context.Context) <-chan client.Result {
	return c.watchF(ctx)
}

func (c *mockClient) Info(_ context.Context) (*chain.Info, error) {
	return c.chainInfo, nil
}

func (c *mockClient) RoundAt(_ time.Time) uint64 {
	return 0
}

func (c *mockClient) Close() error {
	return nil
}

// toRandomDataChain converts the mock results into a chain of client.RandomData
// objects. Note that you do not get back the first result.
func toRandomDataChain(results ...mock.Result) []client.RandomData {
	var randomness []client.RandomData
	prevSig := results[0].Signature()
	for i := 1; i < len(results); i++ {
		randomness = append(randomness, client.RandomData{
			Rnd:               results[i].Round(),
			Random:            results[i].Randomness(),
			Sig:               results[i].Signature(),
			PreviousSignature: prevSig,
		})
		prevSig = results[i].Signature()
	}
	return randomness
}

func tmpDir(t *testing.T) string {
	t.Helper()
	dir, err := ioutil.TempDir(os.TempDir(), "test-gossip-relay-node-datastore")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestWatchRetryOnClose(t *testing.T) {
	chainInfo := &chain.Info{
		Period:      time.Second,
		GenesisTime: time.Now().Unix(),
		PublicKey:   test.GenerateIDs(1)[0].Public.Key,
	}

	results := toRandomDataChain(
		mock.NewMockResult(0),
		mock.NewMockResult(1),
		mock.NewMockResult(2),
		mock.NewMockResult(3),
	)
	wg := sync.WaitGroup{}
	wg.Add(len(results))

	// return a channel that writes one result then closes
	watchF := func(context.Context) <-chan client.Result {
		ch := make(chan client.Result, 1)
		if len(results) > 0 {
			res := results[0]
			results = results[1:]
			ch <- &res
			wg.Done()
		}
		close(ch)
		return ch
	}

	c := &mockClient{chainInfo, watchF}

	td := tmpDir(t)
	defer func() {
		_ = os.RemoveAll(td)
	}()
	gr, err := NewGossipRelayNode(log.DefaultLogger(), &GossipRelayConfig{
		ChainHash:    hex.EncodeToString(chainInfo.Hash()),
		Addr:         "/ip4/0.0.0.0/tcp/0",
		DataDir:      td,
		IdentityPath: path.Join(td, "identity.key"),
		Client:       c,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Shutdown()
	wg.Wait()

	// even though the watch channel closed, it should have been re-opened by
	// the client multiple times until no results remain.
	if len(results) != 0 {
		t.Fatal("random data items waiting to be consumed", len(results))
	}
}
