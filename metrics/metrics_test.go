package metrics

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestMetricReshare(t *testing.T) {
	mph := func(ctx context.Context) ([]http.Handler, error) {
		return []http.Handler{
			http.RedirectHandler("test", 303),
		}, nil
	}

	l := Start(":0", nil, mph)
	defer l.Close()
	addr := l.Addr()
	resp, err := http.Get(fmt.Sprintf("http://%s/metrics", addr.String()))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatal("req to metrics should succeed.")
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err = client.Get(fmt.Sprintf("http://%s/peer/0", addr.String()))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 303 {
		t.Fatal("lazy reshare didn't do its thing.")
	}

	resp, err = client.Get(fmt.Sprintf("http://%s/peer/1", addr.String()))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 404 {
		t.Fatal("lazy reshare didn't do its thing.")
	}

	resp, err = client.Get(fmt.Sprintf("http://%s/peer/-1", addr.String()))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatal("lazy reshare didn't do its thing.")
	}
}
