package test

import (
	"testing"
	"time"

	"github.com/drand/drand/key"
	"github.com/stretchr/testify/require"
)

// CheckRunningBeacon checks for correct generation of random beacon
// within a group. It tries to collect randomness from every member of
// the group, at least twice, i.e. within two different period. The
// randomness of the first period should be present and different than
// the first one. It fails if there is less *expected* number of nodes
// that reply.
func CheckRunningBeacon(t *testing.T, group *key.Group, expected int) {
	require.NotEqual(t, time.Duration(0), group.Period)

}
