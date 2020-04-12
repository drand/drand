package core

import (
	"testing"
	"time"

	"github.com/drand/drand/test"
	"github.com/stretchr/testify/require"
)

func TestConvertGroup(t *testing.T) {
	_, group := test.BatchIdentities(5)
	group.Period = 5 * time.Second
	group.TransitionTime = time.Now().Unix()
	group.GenesisTime = time.Now().Unix()

	proto := groupToProto(group)
	received, err := ProtoToGroup(proto)
	require.NoError(t, err)
	require.True(t, received.Equal(group))
}
