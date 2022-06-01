package metrics

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestMetricReshare(t *testing.T) {
	mph := func() (map[string]http.Handler, error) {
		m := make(map[string]http.Handler)
		m["test.com"] = http.RedirectHandler("test", http.StatusSeeOther)
		return m, nil
	}

	l := Start(":0", nil, []GroupHandlers{mph})
	defer l.Close()
	addr := l.Addr()
	resp, err := http.Get(fmt.Sprintf("http://%s/metrics", addr.String()))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("req to metrics should succeed. Expected StatusCode: 200, actual: %d", resp.StatusCode)
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
		t.Fatalf("lazy reshare didn't do its thing. Expected StatusCode: 303, actual: %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	resp, err = client.Get(fmt.Sprintf("http://%s/peer/other.com/metrics", addr.String()))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 404 {
		t.Fatalf("lazy reshare didn't do its thing. Expected StatusCode: 404, actual: %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestBuildTimestamp(t *testing.T) {
	buildTimestamp := "29/04/2021@20:23:35"

	reference, err := time.Parse(time.RFC3339, "2021-04-29T20:23:35Z")
	if err != nil {
		t.Fatalf("Error parsing reference time: %s", err)
	}
	expected := reference.Unix()

	actual := getBuildTimestamp(buildTimestamp)
	if actual != expected {
		t.Fatalf("Error converting build timestamp to number. Expected %v, actual %v", expected, actual)
	}
}
