package http

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/drand/drand/client"
	"github.com/drand/drand/client/test/http/mock"
)

func TestHTTPClient(t *testing.T) {
	addr, chainInfo, cancel, _ := mock.NewMockHTTPPublicServer(t, true)
	defer cancel()

	httpClient, err := New("http://"+addr, chainInfo.Hash(), http.DefaultTransport)
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

	if _, err := httpClient.Get(ctx, full.Rnd+1); err != nil {
		t.Fatal("http client should not perform verification of results")
	}
	_ = httpClient.Close()
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
	_ = httpClient.Close()
}

func TestForURLsCreation(t *testing.T) {
	addr, chainInfo, cancel, _ := mock.NewMockHTTPPublicServer(t, false)
	defer cancel()

	clients := ForURLs([]string{"http://invalid.domain/", "http://" + addr}, chainInfo.Hash())
	if len(clients) != 2 {
		t.Fatal("expect both urls returned")
	}
	_ = clients[0].Close()
	_ = clients[1].Close()
}

func TestHTTPWatch(t *testing.T) {
	addr, chainInfo, cancel, _ := mock.NewMockHTTPPublicServer(t, false)
	defer cancel()

	httpClient, err := New("http://"+addr, chainInfo.Hash(), http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := httpClient.Watch(ctx)
	first, ok := <-result
	if !ok {
		t.Fatal("should get a result from watching")
	}
	if len(first.Randomness()) == 0 {
		t.Fatal("should get randomness from watching")
	}
	for range result { // drain the channel until the context expires
	}
	_ = httpClient.Close()
}

func TestHTTPClientClose(t *testing.T) {
	addr, chainInfo, cancel, _ := mock.NewMockHTTPPublicServer(t, false)
	defer cancel()

	httpClient, err := New("http://"+addr, chainInfo.Hash(), http.DefaultTransport)
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
	if err != errClientClosed {
		t.Fatal("unexpected error from closed client", err)
	}

	wg.Wait() // wait for the watch to close
}
