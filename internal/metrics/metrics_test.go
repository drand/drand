package metrics

import (
	"testing"
	"time"
)

// Note that the remote peer metrics are tested in TestMetricsForPeer in cli_test.go

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
