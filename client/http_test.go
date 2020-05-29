package client

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/drand/drand/test/mock"
)

func TestHTTPClient(t *testing.T) {
	addr, chainInfo, cancel := mock.NewMockHTTPPublicServer(t, true)
	defer cancel()

	httpClient, err := NewHTTPClient("http://"+addr, chainInfo.Hash(), http.DefaultTransport)
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
	full, ok := (result).(*RandomData)
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
	addr, chainInfo, cancel := mock.NewMockHTTPPublicServer(t, false)
	defer cancel()

	httpClient, err := NewHTTPClient("http://"+addr, chainInfo.Hash(), http.DefaultTransport)
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
	addr, chainInfo, cancel := mock.NewMockHTTPPublicServer(t, false)
	defer cancel()

	httpClient, err := NewHTTPClient("http://"+addr, chainInfo.Hash(), http.DefaultTransport)
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
	_, ok = <-result
	if ok {
		// Note. there is a second value polled for by the client, but it will
		// be invalid per the mocked grpc backing server.
		t.Fatal("second result should fail per context timeout")
	}
}
