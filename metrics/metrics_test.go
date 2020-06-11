package metrics

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestMetricReshare(t *testing.T) {
	mph := func(ctx context.Context) (map[string]http.Handler, error) {
		m := make(map[string]http.Handler)
		m["test.com"] = http.RedirectHandler("test", http.StatusSeeOther)
		return m, nil
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
	_ = resp.Body.Close()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err = client.Get(fmt.Sprintf("http://%s/peer/test.com/metrics", addr.String()))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 303 {
		t.Fatal("lazy reshare didn't do its thing.")
	}
	_ = resp.Body.Close()

	resp, err = client.Get(fmt.Sprintf("http://%s/peer/other.com/metrics", addr.String()))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 404 {
		t.Fatal("lazy reshare didn't do its thing.")
	}
	_ = resp.Body.Close()
}
