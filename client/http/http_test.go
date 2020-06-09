package http

import (
	"context"
	"net/http"
	nhttp "net/http"
	"testing"
	"time"

	"github.com/drand/drand/client"
	"github.com/drand/drand/client/test/mock"
)

func TestHTTPClient(t *testing.T) {
	addr, chainInfo, cancel, _ := mock.NewMockHTTPPublicServer(t, true)
	defer cancel()

	httpClient, err := New("http://"+addr, chainInfo.Hash(), nhttp.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := httpClient.Get(ctx, 0)
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

	if _, err := httpClient.Get(ctx, full.Rnd+1); err == nil {
		t.Fatal("round n+1 should have an invalid signature")
	}
}

func TestHTTPGetLatest(t *testing.T) {
	addr, chainInfo, cancel, _ := mock.NewMockHTTPPublicServer(t, false)
	defer cancel()

	httpClient, err := New("http://"+addr, chainInfo.Hash(), http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r0, err := httpClient.Get(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r1, err := httpClient.Get(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	if r1.Round() != r0.Round()+1 {
		t.Fatal("expected round progression")
	}
}

func TestHTTPWatch(t *testing.T) {
	addr, chainInfo, cancel, _ := mock.NewMockHTTPPublicServer(t, false)
	defer cancel()

	httpClient, err := New("http://"+addr, chainInfo.Hash(), nhttp.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := httpClient.Watch(ctx)
	first, ok := <-result
	if !ok {
		t.Fatal("Should get a result from watching")
	}
	if len(first.Randomness()) == 0 {
		t.Fatal("should get randomness from watching")
	}
	for range result { // drain the channel until the context expires
	}
}
