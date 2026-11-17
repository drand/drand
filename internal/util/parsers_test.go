package util

import (
	"bytes"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/internal/test"
)

func TestParseGroupFileBytes(t *testing.T) {
	t.Run("empty bytes returns error", func(t *testing.T) {
		g, err := ParseGroupFileBytes(nil)
		assert.Nil(t, g)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty")

		g, err = ParseGroupFileBytes([]byte{})
		assert.Nil(t, g)
		require.Error(t, err)
	})

	t.Run("malformed TOML returns error", func(t *testing.T) {
		g, err := ParseGroupFileBytes([]byte("this is = not valid = toml ["))
		assert.Nil(t, g)
		require.Error(t, err)
	})

	t.Run("valid group round-trips", func(t *testing.T) {
		sch := testScheme(t)
		_, group := test.BatchIdentities(t, 3, sch, "default")

		var buf bytes.Buffer
		require.NoError(t, toml.NewEncoder(&buf).Encode(group.TOML()))

		parsed, err := ParseGroupFileBytes(buf.Bytes())
		require.NoError(t, err)
		require.NotNil(t, parsed)

		assert.Equal(t, group.Threshold, parsed.Threshold)
		assert.Equal(t, group.Period, parsed.Period)
		assert.Equal(t, len(group.Nodes), len(parsed.Nodes))
		assert.Equal(t, group.Scheme.Name, parsed.Scheme.Name)
	})
}
