package client

import (
	"context"
	"testing"
	"time"

	"github.com/drand/drand/key"
)

func TestClientConstraints(t *testing.T) {
	if _, e := New(); e == nil {
		t.Fatal("client can't be created without root of trust")
	}

	if _, e := New(WithGroupHash([]byte{0})); e == nil {
		t.Fatal("Client needs URLs if only a group hash is specified")
	}

	if _, e := New(WithHTTPEndpoints([]string{"http://test.com"})); e == nil {
		t.Fatal("Client needs root of trust unless insecure specified explicitly")
	}

	addr, _, cancel := withServer(t)
	defer cancel()

	if _, e := New(WithInsecureHTTPEndpoints([]string{"http://" + addr})); e != nil {
		t.Fatal(e)
	}
}

func TestClientMultiple(t *testing.T) {
	addr1, hash, cancel := withServer(t)
	defer cancel()
	addr2, _, cancel2 := withServer(t)
	defer cancel2()

	c, e := New(WithHTTPEndpoints([]string{"http://" + addr1, "http://" + addr2}), WithGroupHash(hash))
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

func TestClientWithGroup(t *testing.T) {
	c, err := New(WithGroup(key.NewGroup([]*key.Identity{}, 1, 100, time.Second)), WithHTTPEndpoints([]string{"http://nxdomain.local/"}))
	if err != nil {
		t.Fatal("existing group creation shouldn't do additional validaiton.")
	}
	_, err = c.Get(context.Background(), 0)
	if err == nil {
		t.Fatal("bad urls should clearly not provide randomness.")
	}
}

func TestClientCache(t *testing.T) {
	addr1, hash, cancel := withServer(t)
	defer cancel()

	c, e := New(WithHTTPEndpoints([]string{"http://" + addr1}), WithGroupHash(hash), WithCacheSize(1))
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.Get(context.Background(), 0)
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	_, e = c.Get(context.Background(), 0)
	if e != nil {
		t.Fatal(e)
	}

	_, e = c.Get(context.Background(), 4)
	if e == nil {
		t.Fatal("non-cached results should fail.")
	}

}

func TestClientWithoutCache(t *testing.T) {
	addr1, hash, cancel := withServer(t)
	defer cancel()

	c, e := New(WithHTTPEndpoints([]string{"http://" + addr1}), WithGroupHash(hash), WithCacheSize(0))
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
