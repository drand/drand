package http_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/AnomalRoil/syncclock"
	json "github.com/nikkolasg/hexjson"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common/client"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/common/testlogger"
	"github.com/drand/drand/v2/crypto"
	dhttp "github.com/drand/drand/v2/handler/http"
	"github.com/drand/drand/v2/internal/test"
	"github.com/drand/drand/v2/test/mock"
)

func withClient(t *testing.T, clk *syncclock.SyncClock) (client.Client, func(bool)) {
	t.Helper()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	s := mock.NewMockServer(t, false, sch, clk)
	c := mock.NewGrpcClient(s.(*mock.Server))

	service := s.(mock.Service)
	return c, service.EmitRand
}

func validateBodyFormat(respBody io.Reader, round float64) error {
	body := make(map[string]interface{})
	if err := json.NewDecoder(respBody).Decode(&body); err != nil {
		return err
	}
	if len(body) != 4 && len(body) != 3 {
		return fmt.Errorf("beacon formatting expects 3 or 4 fields for the beacon")
	}
	if rep, ok := body["round"].(float64); !ok || rep != round {
		return fmt.Errorf("wrong response round number or format (!%f): %v", round, body)
	}
	if rep, ok := body["randomness"].(string); !ok || len(rep) != 64 {
		return fmt.Errorf("wrong randomness format (!%f): %v", round, body)
	}
	if rep, ok := body["signature"].(string); !ok || len(rep) < 96 {
		return fmt.Errorf("wrong signature format (!%f): %v", round, body)
	}
	if rep, ok := body["previous_signature"].(string); len(body) == 4 && (!ok || len(rep) < 10) {
		return fmt.Errorf("wrong previous_signature (!%f): %v", round, body)
	}
	return nil
}

func TestHTTPWaiting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		lg := testlogger.New(t)

		ctx := log.ToContext(context.Background(), lg)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		test.Tracer(t, ctx)

		clk := syncclock.NewFakeClockAt(time.Now())

		// Use NewMockServer (no gRPC listener) and call the HTTP handler
		// directly via httptest to avoid non-durable IO wait goroutines that
		// prevent syncclock.Advance (time.Sleep) from completing inside the
		// synctest bubble.
		c, push := withClient(t, clk)

		handler, err := dhttp.New(ctx, "")
		require.NoError(t, err)

		info, err := c.Info(ctx)
		require.NoError(t, err)

		handler.RegisterNewBeaconHandler(c, info.HashString())
		h := handler.GetHTTPHandler()

		// The first request will trigger background watch. 1 get (1969)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/"+info.HashString()+"/public/1", nil)
		h.ServeHTTP(rec, req)
		resp := rec.Result()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, validateBodyFormat(resp.Body, 1969))

		// Ensure the PublicRandStream goroutine has set s.stream on the mock
		// before we try to emit through it.
		synctest.Wait()

		// 1 watch get will occur (1970 - the bad one)
		push(false)

		// the following tests when the request is among the pending ones and get released when it's emitted
		done := make(chan time.Time)
		before := clk.Now()
		go func() {
			t.Logf("next request")
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/"+info.HashString()+"/public/1971", nil)
			h.ServeHTTP(rec, req)
			resp := rec.Result()
			if resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("unexpected status: %v", resp.StatusCode)
				done <- time.Unix(0, 0)
				return
			}
			require.NoError(t, validateBodyFormat(resp.Body, 1971))
			done <- clk.Now()
		}()

		synctest.Wait()

		// we emit the request after having requested it
		push(false)
		push(true)

		var after time.Time
		select {
		// it should have arrived as soon as it's emitted
		case x := <-done:
			require.NoError(t, err)
			after = x
		case <-time.After(10 * time.Millisecond):
			t.Fatal("should return after a round")
		}

		t.Logf("comparing values: before: %d after: %d\n", before.Unix(), after.Unix())

		// mock grpc server spits out new round every second on streaming interface.
		if after.Sub(before) > time.Second {
			t.Fatalf("unexpected timing to receive response: before: %s after: %s", before, after)
		}
	})
}

func TestHTTPWatchFuture(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		lg := testlogger.New(t)
		ctx := log.ToContext(context.Background(), lg)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		clk := syncclock.NewFakeClockAt(time.Now())
		c, push := withClient(t, clk)

		test.Tracer(t, ctx)

		handler, err := dhttp.New(ctx, "")
		require.NoError(t, err)

		info, err := c.Info(ctx)
		require.NoError(t, err)

		handler.RegisterNewBeaconHandler(c, info.HashString())
		h := handler.GetHTTPHandler()

		// Trigger background watch by making an initial request.
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/"+info.HashString()+"/public/1", nil)
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Result().StatusCode)

		synctest.Wait()

		// watching sets latest round, future rounds should become inaccessible.
		rec = httptest.NewRecorder()
		req = httptest.NewRequest("GET", "/"+info.HashString()+"/public/2000", nil)
		h.ServeHTTP(rec, req)
		if rec.Result().StatusCode != http.StatusNotFound {
			t.Fatal("response should fail on requests in the future")
		}

		// Close the stream to unblock the background watch goroutine before the bubble exits.
		push(true)
		synctest.Wait()
	})
}

func TestHTTPHealth(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		lg := testlogger.New(t)
		ctx := log.ToContext(context.Background(), lg)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		test.Tracer(t, ctx)

		clk := syncclock.NewFakeClockAt(time.Now())
		c, push := withClient(t, clk)

		handler, err := dhttp.New(ctx, "")
		require.NoError(t, err)

		info, err := c.Info(ctx)
		require.NoError(t, err)

		handler.RegisterNewBeaconHandler(c, info.HashString())
		h := handler.GetHTTPHandler()

		// Newly started server not expected to be synced.
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/"+info.HashString()+"/health", nil)
		h.ServeHTTP(rec, req)
		require.NotEqual(t, http.StatusOK, rec.Result().StatusCode, "newly started server not expected to be synced.")

		// First public request triggers background watch.
		rec = httptest.NewRecorder()
		req = httptest.NewRequest("GET", "/"+info.HashString()+"/public/0", nil)
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Result().StatusCode, "startup of the server on 1st request should happen")

		// Wait for the watch stream to be established before emitting.
		synctest.Wait()

		push(false)

		synctest.Wait()

		rec = httptest.NewRecorder()
		req = httptest.NewRequest("GET", "/"+info.HashString()+"/health", nil)
		h.ServeHTTP(rec, req)
		result := rec.Result()
		buf, err := io.ReadAll(result.Body)
		require.NoError(t, err)
		require.Equalf(t, http.StatusOK, result.StatusCode, "after start server expected to be healthy relatively quickly. %v - %v", string(buf), result.StatusCode)

		// Close the stream to unblock the background watch goroutine before the bubble exits.
		push(true)
		synctest.Wait()
	})
}

func TestHTTP404(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		clk := syncclock.NewFakeClockAt(time.Now())
		c, _ := withClient(t, clk)

		handler, err := dhttp.New(ctx, "")
		require.NoError(t, err)

		info, err := c.Info(ctx)
		require.NoError(t, err)

		handler.RegisterNewBeaconHandler(c, info.HashString())
		h := handler.GetHTTPHandler()

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/deadbeef/public/latest", nil)
		h.ServeHTTP(rec, req)
		if rec.Result().StatusCode != http.StatusNotFound {
			t.Fatal("response should 404 on beacon hash that doesn't exist")
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest("GET", "/deadbeef/public/1", nil)
		h.ServeHTTP(rec, req)
		if rec.Result().StatusCode != http.StatusNotFound {
			t.Fatal("response should 404 on beacon hash that doesn't exist")
		}
	})
}
