package http

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	clock "github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/client"
	"github.com/drand/drand/client/test/http/mock"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/internal/test/testlogger"
)

func TestHTTPClient(t *testing.T) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr, chainInfo, cancel, _ := mock.NewMockHTTPPublicServer(t, true, sch, clk)
	defer cancel()

	err = IsServerReady(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	l := testlogger.New(t)
	httpClient, err := New(ctx, l, "http://"+addr, chainInfo.Hash(), http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}

	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()
	result, err := httpClient.Get(ctx1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Randomness()) == 0 {
		t.Fatal("no randomness provided")
	}
	full, ok := (result).(*client.RandomData)
	if !ok {
		t.Fatal("Should be able to restore concrete type")
	}
	if len(full.Sig) == 0 {
		t.Fatal("no signature provided")
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	if _, err := httpClient.Get(ctx2, full.Rnd+1); err != nil {
		t.Fatalf("http client should not perform verification of results. err: %s", err)
	}
	_ = httpClient.Close()
}

func TestHTTPGetLatest(t *testing.T) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr, chainInfo, cancel, _ := mock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	err = IsServerReady(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	l := testlogger.New(t)
	httpClient, err := New(ctx, l, "http://"+addr, chainInfo.Hash(), http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel = context.WithTimeout(ctx, time.Second)
	defer cancel()
	r0, err := httpClient.Get(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel = context.WithTimeout(ctx, time.Second)
	defer cancel()
	r1, err := httpClient.Get(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	if r1.Round() != r0.Round()+1 {
		t.Fatal("expected round progression")
	}
	_ = httpClient.Close()
}

func TestForURLsCreation(t *testing.T) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr, chainInfo, cancel, _ := mock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	err = IsServerReady(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	l := testlogger.New(t)
	clients := ForURLs(ctx, l, []string{"http://invalid.domain/", "http://" + addr}, chainInfo.Hash())
	if len(clients) != 2 {
		t.Fatal("expect both urls returned")
	}
	_ = clients[0].Close()
	_ = clients[1].Close()
}

func TestHTTPWatch(t *testing.T) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr, chainInfo, cancel, _ := mock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	err = IsServerReady(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	l := testlogger.New(t)
	httpClient, err := New(ctx, l, "http://"+addr, chainInfo.Hash(), http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := httpClient.Watch(ctx)
	first, ok := <-result
	if !ok {
		t.Fatal("should get a result from watching")
	}
	if len(first.Randomness()) == 0 {
		t.Fatal("should get randomness from watching")
	}

	for range result {
	}
	_ = httpClient.Close()
}

func TestHTTPClientClose(t *testing.T) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr, chainInfo, cancel, _ := mock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	err = IsServerReady(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	l := testlogger.New(t)
	httpClient, err := New(ctx, l, "http://"+addr, chainInfo.Hash(), http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}
	result, err := httpClient.Get(context.Background(), 1969)
	if err != nil {
		t.Fatal(err)
	}
	if result.Round() != 1969 {
		t.Fatal("unexpected round.")
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		for range httpClient.Watch(context.Background()) {
		}
		wg.Done()
	}()

	err = httpClient.Close()
	if err != nil {
		t.Fatal(err)
	}

	_, err = httpClient.Get(context.Background(), 0)
	if !errors.Is(err, errClientClosed) {
		t.Fatal("unexpected error from closed client", err)
	}

	wg.Wait() // wait for the watch to close
}
