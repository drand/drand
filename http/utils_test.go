package http

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/drand/drand/client/http"
)

const maxHTTPServerTries = 10

func WaitServerToBeReady(t *testing.T, addr string) error {
	counter := 0

	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		err := http.Ping(ctx, "http://"+addr)
		if err == nil {
			t.Log("Http server is ready to attend requests")
			return nil
		}

		counter++
		if counter == maxHTTPServerTries {
			return fmt.Errorf("timeout waiting http server to be ready")
		}

		t.Log("Http server is not ready yet. We will check it again.")
		time.Sleep(1 * time.Second)
	}
}
