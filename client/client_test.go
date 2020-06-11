package client_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/client/http"
	cmock "github.com/drand/drand/client/test/mock"
	"github.com/drand/drand/test"
)

func TestClientConstraints(t *testing.T) {
	if _, e := client.New(); e == nil {
		t.Fatal("client can't be created without root of trust")
	}

	if _, e := client.New(client.WithChainHash([]byte{0})); e == nil {
		t.Fatal("Client needs URLs if only a chain hash is specified")
	}

	if _, e := client.New(client.From(client.MockClientWithResults(0, 5))); e == nil {
		t.Fatal("Client needs root of trust unless insecure specified explicitly")
	}

	if _, e := client.New(client.From(client.MockClientWithResults(0, 5)), client.Insecurely()); e != nil {
		t.Fatal(e)
	}
}

func TestClientMultiple(t *testing.T) {
	addr1, chainInfo, cancel, _ := cmock.NewMockHTTPPublicServer(t, false)
	defer cancel()
	addr2, _, cancel2, _ := cmock.NewMockHTTPPublicServer(t, false)
	defer cancel2()

	c, e := client.New(
		client.From(http.ForURLs([]string{"http://" + addr1, "http://" + addr2}, chainInfo.Hash())...),
		client.WithChainHash(chainInfo.Hash()))
	if e != nil {
		t.Fatal(e)
	}
	r, e := c.Get(context.Background(), 0)
	if e != nil {
		t.Fatal(e)
	}
	if r.Round() <= 0 {
		t.Fatal("expected valid client")
	}
}

func TestClientWithChainInfo(t *testing.T) {
	id := test.GenerateIDs(1)[0]
	chainInfo := &chain.Info{
		PublicKey:   id.Public.Key,
		GenesisTime: 100,
		Period:      time.Second,
	}
	hc, _ := http.NewWithInfo("http://nxdomain.local/", chainInfo, nil)
	c, err := client.New(client.WithChainInfo(chainInfo),
		client.From(hc))
	if err != nil {
		t.Fatal("existing group creation shouldn't do additional validaiton.")
	}
	_, err = c.Get(context.Background(), 0)
	if err == nil {
		t.Fatal("bad urls should clearly not provide randomness.")
	}
}

func TestClientCache(t *testing.T) {
	addr1, chainInfo, cancel, _ := cmock.NewMockHTTPPublicServer(t, false)
	defer cancel()

	c, e := client.New(client.From(http.ForURLs([]string{"http://" + addr1}, chainInfo.Hash())...),
		client.WithChainHash(chainInfo.Hash()), client.WithCacheSize(1))
	if e != nil {
		t.Fatal(e)
	}
	r0, e := c.Get(context.Background(), 0)
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	_, e = c.Get(context.Background(), r0.Round())
	if e != nil {
		t.Fatal(e)
	}

	_, e = c.Get(context.Background(), 4)
	if e == nil {
		t.Fatal("non-cached results should fail.")
	}
}

func TestClientWithoutCache(t *testing.T) {
	addr1, chainInfo, cancel, _ := cmock.NewMockHTTPPublicServer(t, false)
	defer cancel()

	c, e := client.New(
		client.From(http.ForURLs([]string{"http://" + addr1}, chainInfo.Hash())...),
		client.WithChainHash(chainInfo.Hash()),
		client.WithCacheSize(0))
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.Get(context.Background(), 0)
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	_, e = c.Get(context.Background(), 0)
	if e == nil {
		t.Fatal("cache should be disabled.")
	}
}

func TestClientWithFailover(t *testing.T) {
	addr1, chainInfo, cancel, _ := cmock.NewMockHTTPPublicServer(t, false)
	defer cancel()

	// ensure a client with failover can be created successfully without error
	_, err := client.New(
		client.From(http.ForURLs([]string{"http://" + addr1}, chainInfo.Hash())...),
		client.WithChainHash(chainInfo.Hash()),
		client.WithFailoverGracePeriod(time.Second*5),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientWithWatcher(t *testing.T) {
	addr1, chainInfo, cancel, _ := cmock.NewMockHTTPPublicServer(t, false)
	defer cancel()

	results := []client.MockResult{
		{Rnd: 1, Rand: []byte{1}},
		{Rnd: 2, Rand: []byte{2}},
	}

	ch := make(chan client.Result, len(results))
	for i := range results {
		ch <- &results[i]
	}
	close(ch)

	watcherCtor := func(chainInfo *chain.Info, _ client.Cache) (client.Watcher, error) {
		return &client.MockClient{WatchCh: ch}, nil
	}

	c, err := client.New(
		client.From(http.ForURLs([]string{"http://" + addr1}, chainInfo.Hash())...),
		client.WithChainHash(chainInfo.Hash()),
		client.WithWatcher(watcherCtor),
	)
	if err != nil {
		t.Fatal(err)
	}

	i := 0
	for r := range c.Watch(context.Background()) {
		compareResults(t, r, &results[i])
		i++
	}
}

func TestClientAutoWatch(t *testing.T) {
	addr1, chainInfo, cancel, _ := cmock.NewMockHTTPPublicServer(t, false)
	defer cancel()

	results := []client.MockResult{
		{Rnd: 1, Rand: []byte{1}},
		{Rnd: 2, Rand: []byte{2}},
	}

	ch := make(chan client.Result, len(results))
	for i := range results {
		ch <- &results[i]
	}
	close(ch)

	watcherCtor := func(chainInfo *chain.Info, _ client.Cache) (client.Watcher, error) {
		return &client.MockClient{WatchCh: ch}, nil
	}

	c, err := client.New(
		client.From(http.ForURLs([]string{"http://" + addr1}, chainInfo.Hash())...),
		client.WithChainHash(chainInfo.Hash()),
		client.WithWatcher(watcherCtor),
		client.WithAutoWatch(),
	)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(chainInfo.Period)
	cancel()
	r, err := c.Get(context.Background(), results[0].Round())
	if err != nil {
		t.Fatal(err)
	}
	compareResults(t, r, &results[0])
}

// compareResults asserts that two results are the same.
func compareResults(t *testing.T, a, b client.Result) {
	t.Helper()

	if a.Round() != b.Round() {
		t.Fatal("unexpected result round", a.Round(), b.Round())
	}
	if !bytes.Equal(a.Randomness(), b.Randomness()) {
		t.Fatal("unexpected result randomness", a.Randomness(), b.Randomness())
	}
}
