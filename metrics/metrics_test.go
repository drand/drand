package metrics

import (
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
	"time"

	"github.com/drand/drand/common"
)

func TestMetricReshare(t *testing.T) {
	calls := make(map[string]int)

	calls["test.com"] = 0
	calls["error.com"] = 0
	calls["nogroup.com"] = 0
	calls["undefined"] = 0

	hdl := func(addr string) (http.Handler, error) {
		switch addr {
		case "test.com":
			calls["test.com"]++
			return http.RedirectHandler("test", http.StatusSeeOther), nil
		case "error.com":
			calls["error.com"]++
			return nil, errors.New("some random error")
		default:
			calls["undefined"]++
			return nil, fmt.Errorf("%w for Beacon ID: test_beacon", common.ErrNotPartOfGroup)
		}
	}

	l := Start(":0", nil, []Handler{hdl})
	defer l.Close()
	addr := l.Addr()
	resp, err := http.Get(fmt.Sprintf("http://%s/metrics", addr.String()))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
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
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("lazy reshare didn't do its thing. Expected StatusCode: 303, actual: %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	resp, err = client.Get(fmt.Sprintf("http://%s/peer/test.com/metrics", addr.String()))
	if err != nil {
		t.Fatal(err)
	}
	if calls["test.com"] != 1 {
		t.Fatalf("Handler function called %d times for test.com - caching is not working", calls["test.com"])
	}
	_ = resp.Body.Close()

	resp, err = client.Get(fmt.Sprintf("http://%s/peer/other.com/metrics", addr.String()))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("lazy reshare didn't do its thing. Expected StatusCode: 404, actual: %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	resp, err = client.Get(fmt.Sprintf("http://%s/peer/error.com/metrics", addr.String()))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("error not propagated correctly from error.com. Expected StatusCode: 303, actual: %d", resp.StatusCode)
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

func TestCreatingMetricHandlerWithoutHandlerDoesntPanic(t *testing.T) {
	funcReturningNil := func(_ string) (http.Handler, error) {
		return nil, nil
	}
	h := newLazyPeerHandler([]Handler{funcReturningNil})

	_, err := h.handlerForPeer("127.0.0.1")

	require.Error(t, err)
}
