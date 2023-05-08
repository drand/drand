package client_test

import (
	"context"
	"errors"
	"testing"
	"time"

	client2 "github.com/drand/drand/client"
	"github.com/drand/drand/client/http"
	clientMock "github.com/drand/drand/client/mock"
	httpmock "github.com/drand/drand/client/test/http/mock"
	"github.com/drand/drand/client/test/result/mock"
	"github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/client"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/internal/test"
	"github.com/drand/drand/internal/test/testlogger"
	clock "github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
)

func TestClientConstraints(t *testing.T) {
	ctx := context.Background()
	lg := testlogger.New(t)
	if _, e := client2.New(ctx, lg); e == nil {
		t.Fatal("client can't be created without root of trust")
	}

	if _, e := client2.New(ctx, lg, client2.WithChainHash([]byte{0})); e == nil {
		t.Fatal("Client needs URLs if only a chain hash is specified")
	}

	if _, e := client2.New(ctx, lg, client2.From(clientMock.ClientWithResults(0, 5))); e == nil {
		t.Fatal("Client needs root of trust unless insecure specified explicitly")
	}

	c := clientMock.ClientWithResults(0, 5)
	// As we will run is insecurely, we will set chain info so client can fetch it
	c.OptionalInfo = fakeChainInfo(t)

	if _, e := client2.New(ctx, lg, client2.From(c), client2.Insecurely()); e != nil {
		t.Fatal(e)
	}
}

func TestClientMultiple(t *testing.T) {
	ctx := context.Background()
	lg := testlogger.New(t)
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr1, chainInfo, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	addr2, _, cancel2, _ := httpmock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel2()

	httpClients := http.ForURLs(ctx, lg, []string{"http://" + addr1, "http://" + addr2}, chainInfo.Hash())
	if len(httpClients) == 0 {
		t.Error("http clients is empty")
		return
	}

	var c client.Client
	var e error
	c, e = client2.New(ctx,
		lg,
		client2.From(httpClients...),
		client2.WithChainHash(chainInfo.Hash()))

	if e != nil {
		t.Fatal(e)
	}
	r, e := c.Get(ctx, 0)
	if e != nil {
		t.Fatal(e)
	}
	if r.Round() <= 0 {
		t.Fatal("expected valid client")
	}
	_ = c.Close()
}

func TestClientWithChainInfo(t *testing.T) {
	ctx := context.Background()
	id := test.GenerateIDs(1)[0]
	chainInfo := &chain.Info{
		PublicKey:   id.Public.Key,
		GenesisTime: 100,
		Period:      time.Second,
		Scheme:      crypto.DefaultSchemeID,
	}
	lg := testlogger.New(t)
	hc, err := http.NewWithInfo(lg, "http://nxdomain.local/", chainInfo, nil)
	require.NoError(t, err)
	c, err := client2.New(ctx, lg, client2.WithChainInfo(chainInfo),
		client2.From(hc))
	if err != nil {
		t.Fatal("existing group creation shouldn't do additional validaiton.")
	}
	_, err = c.Get(ctx, 0)
	if err == nil {
		t.Fatal("bad urls should clearly not provide randomness.")
	}
	_ = c.Close()
}

func TestClientCache(t *testing.T) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr1, chainInfo, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	lg := testlogger.New(t)
	httpClients := http.ForURLs(ctx, lg, []string{"http://" + addr1}, chainInfo.Hash())
	if len(httpClients) == 0 {
		t.Error("http clients is empty")
		return
	}

	var c client.Client
	var e error
	c, e = client2.New(ctx, lg, client2.From(httpClients...),
		client2.WithChainHash(chainInfo.Hash()), client2.WithCacheSize(1))

	if e != nil {
		t.Fatal(e)
	}
	r0, e := c.Get(ctx, 0)
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	_, e = c.Get(ctx, r0.Round())
	if e != nil {
		t.Fatal(e)
	}

	_, e = c.Get(ctx, 4)
	if e == nil {
		t.Fatal("non-cached results should fail.")
	}
	_ = c.Close()
}

func TestClientWithoutCache(t *testing.T) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr1, chainInfo, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	lg := testlogger.New(t)
	httpClients := http.ForURLs(ctx, lg, []string{"http://" + addr1}, chainInfo.Hash())
	if len(httpClients) == 0 {
		t.Error("http clients is empty")
		return
	}

	var c client.Client
	var e error
	c, e = client2.New(ctx,
		lg,
		client2.From(httpClients...),
		client2.WithChainHash(chainInfo.Hash()),
		client2.WithCacheSize(0))

	if e != nil {
		t.Fatal(e)
	}
	_, e = c.Get(ctx, 0)
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	_, e = c.Get(ctx, 0)
	if e == nil {
		t.Fatal("cache should be disabled.")
	}
	_ = c.Close()
}

func TestClientWithWatcher(t *testing.T) {
	ctx := context.Background()
	lg := testlogger.New(t)
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	info, results := mock.VerifiableResults(2, sch)

	ch := make(chan client.Result, len(results))
	for i := range results {
		ch <- &results[i]
	}
	close(ch)

	watcherCtor := func(chainInfo *chain.Info, _ client2.Cache) (client2.Watcher, error) {
		return &clientMock.Client{WatchCh: ch}, nil
	}

	var c client.Client
	c, err = client2.New(ctx,
		lg,
		client2.WithChainInfo(info),
		client2.WithWatcher(watcherCtor),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	w := c.Watch(ctx)

	for i := 0; i < len(results); i++ {
		r := <-w
		compareResults(t, &results[i], r)
	}
	require.NoError(t, c.Close())
}

func TestClientWithWatcherCtorError(t *testing.T) {
	ctx := context.Background()
	lg := testlogger.New(t)
	watcherErr := errors.New("boom")
	watcherCtor := func(chainInfo *chain.Info, _ client2.Cache) (client2.Watcher, error) {
		return nil, watcherErr
	}

	// constructor should return error returned by watcherCtor
	_, err := client2.New(ctx,
		lg,
		client2.WithChainInfo(fakeChainInfo(t)),
		client2.WithWatcher(watcherCtor),
	)
	if !errors.Is(err, watcherErr) {
		t.Fatal(err)
	}
}

func TestClientChainHashOverrideError(t *testing.T) {
	ctx := context.Background()
	lg := testlogger.New(t)
	chainInfo := fakeChainInfo(t)
	_, err := client2.Wrap(
		ctx,
		lg,
		[]client.Client{client2.EmptyClientWithInfo(chainInfo)},
		client2.WithChainInfo(chainInfo),
		client2.WithChainHash(fakeChainInfo(t).Hash()),
	)
	if err == nil {
		t.Fatal("expected error, received no error")
	}
	if err.Error() != "refusing to override group with non-matching hash" {
		t.Fatal(err)
	}
}

func TestClientChainInfoOverrideError(t *testing.T) {
	ctx := context.Background()
	lg := testlogger.New(t)
	chainInfo := fakeChainInfo(t)
	_, err := client2.Wrap(
		ctx,
		lg,
		[]client.Client{client2.EmptyClientWithInfo(chainInfo)},
		client2.WithChainHash(chainInfo.Hash()),
		client2.WithChainInfo(fakeChainInfo(t)),
	)
	if err == nil {
		t.Fatal("expected error, received no error")
	}
	if err.Error() != "refusing to override hash with non-matching group" {
		t.Fatal(err)
	}
}

func TestClientAutoWatch(t *testing.T) {
	ctx := context.Background()
	lg := testlogger.New(t)
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr1, chainInfo, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	httpClient := http.ForURLs(ctx, lg, []string{"http://" + addr1}, chainInfo.Hash())
	if len(httpClient) == 0 {
		t.Error("http clients is empty")
		return
	}

	r1, _ := httpClient[0].Get(ctx, 1)
	r2, _ := httpClient[0].Get(ctx, 2)
	results := []client.Result{r1, r2}

	ch := make(chan client.Result, len(results))
	for i := range results {
		ch <- results[i]
	}
	close(ch)

	watcherCtor := func(chainInfo *chain.Info, _ client2.Cache) (client2.Watcher, error) {
		return &clientMock.Client{WatchCh: ch}, nil
	}

	var c client.Client
	c, err = client2.New(ctx,
		lg,
		client2.From(clientMock.ClientWithInfo(chainInfo)),
		client2.WithChainHash(chainInfo.Hash()),
		client2.WithWatcher(watcherCtor),
		client2.WithAutoWatch(),
	)

	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(chainInfo.Period)
	cancel()
	r, err := c.Get(ctx, results[0].Round())
	if err != nil {
		t.Fatal(err)
	}
	compareResults(t, r, results[0])
	_ = c.Close()
}

func TestClientAutoWatchRetry(t *testing.T) {
	ctx := context.Background()
	lg := testlogger.New(t)
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	info, results := mock.VerifiableResults(5, sch)
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

	var failer clientMock.Client
	failer = clientMock.Client{
		WatchF: func(ctx context.Context) <-chan client.Result {
			// First call returns a closed channel
			ch := make(chan client.Result)
			close(ch)
			// Second call returns a channel that writes results
			failer.WatchF = watchF
			return ch
		},
	}

	var c client.Client
	c, err = client2.New(ctx,
		lg,
		client2.From(&failer, clientMock.ClientWithInfo(info)),
		client2.WithChainInfo(info),
		client2.WithAutoWatch(),
		client2.WithAutoWatchRetry(time.Second),
		client2.WithCacheSize(len(results)),
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
		r, err := c.Get(ctx, results[i].Round())
		if err != nil {
			t.Fatal(err)
		}
		compareResults(t, &results[i], r)
	}
}

// compareResults asserts that two results are the same.
func compareResults(t *testing.T, expected, actual client.Result) {
	t.Helper()

	require.NotNil(t, expected)
	require.NotNil(t, actual)
	require.Equal(t, expected.Round(), actual.Round())
	require.Equal(t, expected.Randomness(), actual.Randomness())
}

// fakeChainInfo creates a chain info object for use in tests.
func fakeChainInfo(t *testing.T) *chain.Info {
	t.Helper()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	return &chain.Info{
		Period:      time.Second,
		GenesisTime: time.Now().Unix(),
		PublicKey:   test.GenerateIDs(1)[0].Public.Key,
		Scheme:      sch.Name,
	}
}
