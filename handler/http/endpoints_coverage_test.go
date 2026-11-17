package http_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	clockwork "github.com/jonboulle/clockwork"
	json "github.com/nikkolasg/hexjson"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/client"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/common/testlogger"
	"github.com/drand/drand/v2/crypto"
	dhttp "github.com/drand/drand/v2/handler/http"
	"github.com/drand/drand/v2/test/mock"
)

// newMockClient builds an in-process mock client backed by a clockwork fake
// clock so these tests can run outside a synctest bubble. No gRPC listener and
// no real network is involved.
func newMockClient(t *testing.T) client.Client {
	t.Helper()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	clk := clockwork.NewFakeClock()
	s := mock.NewMockServer(t, false, sch, clk)
	return mock.NewGrpcClient(s.(*mock.Server))
}

// newHandlerWithBeacon builds a DrandHandler backed by an in-process mock
// client, registers it under its real chain hash and returns everything tests
// need. No gRPC listener and no real network is involved.
func newHandlerWithBeacon(t *testing.T) (*dhttp.DrandHandler, http.Handler, string) {
	t.Helper()
	lg := testlogger.New(t)
	ctx := log.ToContext(context.Background(), lg)
	ctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)

	c := newMockClient(t)

	handler, err := dhttp.New(ctx, "testversion")
	require.NoError(t, err)

	info, err := c.Info(ctx)
	require.NoError(t, err)

	handler.RegisterNewBeaconHandler(c, info.HashString())
	return handler, handler.GetHTTPHandler(), info.HashString()
}

func doGET(t *testing.T, h http.Handler, target string) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func TestChainInfoEndpoint(t *testing.T) {
	_, h, hash := newHandlerWithBeacon(t)

	resp := doGET(t, h, "/"+hash+"/info")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	require.Contains(t, resp.Header.Get("Cache-Control"), "immutable")

	body := make(map[string]interface{})
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Contains(t, body, "public_key")
	require.Contains(t, body, "period")
	require.Contains(t, body, "genesis_time")
}

func TestChainInfoDefaultHashRoute(t *testing.T) {
	// the route without an explicit chain hash maps to the "default" beacon,
	// which is not registered here, so it should 404.
	_, h, _ := newHandlerWithBeacon(t)

	resp := doGET(t, h, "/info")
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestChainInfoBadHexChainHash(t *testing.T) {
	_, h, _ := newHandlerWithBeacon(t)

	// "zz" is not valid hex -> readChainHash fails -> 400
	resp := doGET(t, h, "/zz/info")
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestChainInfoUnknownHash(t *testing.T) {
	_, h, _ := newHandlerWithBeacon(t)

	resp := doGET(t, h, "/deadbeef/info")
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestChainHashesEndpoint(t *testing.T) {
	_, h, hash := newHandlerWithBeacon(t)

	resp := doGET(t, h, "/chains")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	require.Equal(t, "max-age=300", resp.Header.Get("Cache-Control"))

	var hashes []string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&hashes))
	require.Contains(t, hashes, hash)
}

func TestChainHashesExcludesDefault(t *testing.T) {
	handler, h, hash := newHandlerWithBeacon(t)

	// Register an additional beacon as the default; it must be excluded from
	// /chains. RegisterNewBeaconHandler returns the *BeaconHandler we then
	// promote to default.
	c2 := newMockClient(t)
	bh := handler.RegisterNewBeaconHandler(c2, common.DefaultChainHash)
	handler.RegisterDefaultBeaconHandler(bh)

	resp := doGET(t, h, "/chains")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var hashes []string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&hashes))
	require.NotContains(t, hashes, common.DefaultChainHash)
	require.Contains(t, hashes, hash)
}

func TestLatestRandEndpoint(t *testing.T) {
	_, h, hash := newHandlerWithBeacon(t)

	resp := doGET(t, h, "/"+hash+"/public/latest")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotEmpty(t, resp.Header.Get("Expires"))
	require.NotEmpty(t, resp.Header.Get("Last-Modified"))

	body := make(map[string]interface{})
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Contains(t, body, "round")
	require.Contains(t, body, "randomness")
}

func TestLatestRandBadHexChainHash(t *testing.T) {
	_, h, _ := newHandlerWithBeacon(t)

	resp := doGET(t, h, "/zz/public/latest")
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestLatestRandUnknownHash(t *testing.T) {
	_, h, _ := newHandlerWithBeacon(t)

	resp := doGET(t, h, "/deadbeef/public/latest")
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestPublicRandRound0RedirectsToLatest(t *testing.T) {
	_, h, hash := newHandlerWithBeacon(t)

	// round 0 is delegated to LatestRand.
	resp := doGET(t, h, "/"+hash+"/public/0")
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestPublicRandBadRound(t *testing.T) {
	_, h, hash := newHandlerWithBeacon(t)

	// "notanumber" can't be parsed as a uint -> 400.
	resp := doGET(t, h, "/"+hash+"/public/notanumber")
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPublicRandBadHexChainHash(t *testing.T) {
	_, h, _ := newHandlerWithBeacon(t)

	resp := doGET(t, h, "/zz/public/5")
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPublicRandUnknownHash(t *testing.T) {
	_, h, _ := newHandlerWithBeacon(t)

	resp := doGET(t, h, "/deadbeef/public/5")
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestHealthBadHexChainHash(t *testing.T) {
	_, h, _ := newHandlerWithBeacon(t)

	resp := doGET(t, h, "/zz/health")
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHealthUnknownHash(t *testing.T) {
	_, h, _ := newHandlerWithBeacon(t)

	resp := doGET(t, h, "/deadbeef/health")
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestCommonHeadersApplied(t *testing.T) {
	_, h, hash := newHandlerWithBeacon(t)

	resp := doGET(t, h, "/"+hash+"/info")
	require.Equal(t, "testversion", resp.Header.Get("Server"))
	require.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestSetHTTPHandler(t *testing.T) {
	handler, _, _ := newHandlerWithBeacon(t)

	custom := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, "custom")
	})
	handler.SetHTTPHandler(custom)

	resp := doGET(t, handler.GetHTTPHandler(), "/anything")
	require.Equal(t, http.StatusTeapot, resp.StatusCode)
}

func TestRemoveBeaconHandler(t *testing.T) {
	handler, h, hash := newHandlerWithBeacon(t)

	// present before removal
	resp := doGET(t, h, "/"+hash+"/info")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	handler.RemoveBeaconHandler(hash)

	// gone after removal -> 404
	resp = doGET(t, h, "/"+hash+"/info")
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}
