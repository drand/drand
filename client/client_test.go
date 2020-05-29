package client

import (
	"context"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/test"
)

func TestClientConstraints(t *testing.T) {
	if _, e := New(); e == nil {
		t.Fatal("client can't be created without root of trust")
	}

	if _, e := New(WithChainHash([]byte{0})); e == nil {
		t.Fatal("Client needs URLs if only a chain hash is specified")
	}

	if _, e := New(WithHTTPEndpoints([]string{"http://test.com"})); e == nil {
		t.Fatal("Client needs root of trust unless insecure specified explicitly")
	}

	addr, _, cancel := withServer(t, false)
	defer cancel()

	if _, e := New(WithInsecureHTTPEndpoints([]string{"http://" + addr})); e != nil {
		t.Fatal(e)
	}
}

func TestClientMultiple(t *testing.T) {
	addr1, hash, cancel := withServer(t, false)
	defer cancel()
	addr2, _, cancel2 := withServer(t, false)
	defer cancel2()

	c, e := New(WithHTTPEndpoints([]string{"http://" + addr1, "http://" + addr2}), WithChainHash(hash))
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
	c, err := New(WithChainInfo(chainInfo), WithHTTPEndpoints([]string{"http://nxdomain.local/"}))
	if err != nil {
		t.Fatal("existing group creation shouldn't do additional validaiton.")
	}
	_, err = c.Get(context.Background(), 0)
	if err == nil {
		t.Fatal("bad urls should clearly not provide randomness.")
	}
}

func TestClientCache(t *testing.T) {
	addr1, hash, cancel := withServer(t, false)
	defer cancel()

	c, e := New(WithHTTPEndpoints([]string{"http://" + addr1}), WithChainHash(hash), WithCacheSize(1))
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
	addr1, hash, cancel := withServer(t, false)
	defer cancel()

	c, e := New(WithHTTPEndpoints([]string{"http://" + addr1}), WithChainHash(hash), WithCacheSize(0))
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
	addr1, hash, cancel := withServer(t, false)
	defer cancel()

	// ensure a client with failover can be created successfully without error
	_, err := New(
		WithHTTPEndpoints([]string{"http://" + addr1}),
		WithChainHash(hash),
		WithFailoverGracePeriod(time.Second*5),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientWithWatcher(t *testing.T) {
	addr1, hash, cancel := withServer(t, false)
	defer cancel()

	results := []MockResult{
		{rnd: 1, rand: []byte{1}},
		{rnd: 2, rand: []byte{2}},
	}

	ch := make(chan Result, len(results))
	for i := range results {
		ch <- &results[i]
	}
	close(ch)

	watcherCtor := func(chainInfo *chain.Info, _ Cache) (Watcher, error) {
		return &MockClient{WatchCh: ch}, nil
	}

	c, err := New(
		WithHTTPEndpoints([]string{"http://" + addr1}),
		WithChainHash(hash),
		WithWatcher(watcherCtor),
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
