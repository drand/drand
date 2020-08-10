package client_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/client/http"
	httpmock "github.com/drand/drand/client/test/http/mock"
	"github.com/drand/drand/client/test/result/mock"
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
	addr1, chainInfo, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false)
	defer cancel()
	addr2, _, cancel2, _ := httpmock.NewMockHTTPPublicServer(t, false)
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
	_ = c.Close()
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
	_ = c.Close()
}

func TestClientCache(t *testing.T) {
	addr1, chainInfo, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false)
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
	_ = c.Close()
}

func TestClientWithoutCache(t *testing.T) {
	addr1, chainInfo, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false)
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
	_ = c.Close()
}

func TestClientWithWatcher(t *testing.T) {
	info, results := mock.VerifiableResults(2)

	ch := make(chan client.Result, len(results))
	for i := range results {
		ch <- &results[i]
	}
	close(ch)

	watcherCtor := func(chainInfo *chain.Info, _ client.Cache) (client.Watcher, error) {
		return &client.MockClient{WatchCh: ch}, nil
	}

	c, err := client.New(
		client.WithChainInfo(info),
		client.WithWatcher(watcherCtor),
	)
	if err != nil {
		t.Fatal(err)
	}

	i := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for r := range c.Watch(ctx) {
		compareResults(t, r, &results[i])
		i++
		if i == len(results) {
			break
		}
	}
	_ = c.Close()
}

func TestClientWithWatcherCtorError(t *testing.T) {
	watcherErr := errors.New("boom")
	watcherCtor := func(chainInfo *chain.Info, _ client.Cache) (client.Watcher, error) {
		return nil, watcherErr
	}

	// constructor should return error returned by watcherCtor
	_, err := client.New(
		client.WithChainInfo(fakeChainInfo()),
		client.WithWatcher(watcherCtor),
	)
	if err != watcherErr {
		t.Fatal(err)
	}
}

func TestClientChainHashOverrideError(t *testing.T) {
	chainInfo := fakeChainInfo()
	_, err := client.Wrap(
		[]client.Client{client.EmptyClientWithInfo(chainInfo)},
		client.WithChainInfo(chainInfo),
		client.WithChainHash(fakeChainInfo().Hash()),
	)
	if err.Error() != "refusing to override group with non-matching hash" {
		t.Fatal(err)
	}
}

func TestClientChainInfoOverrideError(t *testing.T) {
	chainInfo := fakeChainInfo()
	_, err := client.Wrap(
		[]client.Client{client.EmptyClientWithInfo(chainInfo)},
		client.WithChainHash(chainInfo.Hash()),
		client.WithChainInfo(fakeChainInfo()),
	)
	if err.Error() != "refusing to override hash with non-matching group" {
		t.Fatal(err)
	}
}

func TestClientAutoWatch(t *testing.T) {
	addr1, chainInfo, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false)
	defer cancel()

	httpClient := http.ForURLs([]string{"http://" + addr1}, chainInfo.Hash())
	r1, _ := httpClient[0].Get(context.Background(), 1)
	r2, _ := httpClient[0].Get(context.Background(), 2)
	results := []client.Result{r1, r2}

	ch := make(chan client.Result, len(results))
	for i := range results {
		ch <- results[i]
	}
	close(ch)

	watcherCtor := func(chainInfo *chain.Info, _ client.Cache) (client.Watcher, error) {
		return &client.MockClient{WatchCh: ch}, nil
	}

	c, err := client.New(
		client.From(client.MockClientWithInfo(chainInfo)),
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
	compareResults(t, r, results[0])
	_ = c.Close()
}

func TestClientAutoWatchRetry(t *testing.T) {
	info, results := mock.VerifiableResults(5)
	resC := make(chan client.Result)
	defer close(resC)

	// done is closed after all resuls have been written to resC
	done := make(chan struct{})

	// Returns a channel that yields the verifiable results above
	watchF := func(ctx context.Context) <-chan client.Result {
		go func() {
			for i := 0; i < len(results); i++ {
				select {
				case resC <- &results[i]:
				case <-ctx.Done():
					return
				}
			}
			<-time.After(time.Second)
			close(done)
		}()
		return resC
	}

	var failer client.MockClient
	failer = client.MockClient{
		WatchF: func(ctx context.Context) <-chan client.Result {
			// First call returns a closed channel
			ch := make(chan client.Result)
			close(ch)
			// Second call returns a channel that writes results
			failer.WatchF = watchF
			return ch
		},
	}

	c, err := client.New(
		client.From(&failer, client.MockClientWithInfo(info)),
		client.WithChainInfo(info),
		client.WithAutoWatch(),
		client.WithAutoWatchRetry(time.Second),
		client.WithCacheSize(len(results)),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Wait for all the results to be consumed by the autoWatch
	select {
	case <-done:
	case <-time.After(time.Minute):
		t.Fatal("timed out waiting for results to be consumed")
	}

	// We should be able to retrieve all the results from the cache.
	for i := range results {
		r, err := c.Get(context.Background(), results[i].Round())
		if err != nil {
			t.Fatal(err)
		}
		compareResults(t, &results[i], r)
	}
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

// fakeChainInfo creates a chain info object for use in tests.
func fakeChainInfo() *chain.Info {
	return &chain.Info{
		Period:      time.Second,
		GenesisTime: time.Now().Unix(),
		PublicKey:   test.GenerateIDs(1)[0].Public.Key,
	}
}
