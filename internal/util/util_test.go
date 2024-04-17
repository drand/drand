package util

import (
	"testing"

	drand "github.com/drand/drand/v2/protobuf/dkg"
	"github.com/stretchr/testify/assert"
)

func TestWithout(t *testing.T) {
	t.Run("empty haystack", func(st *testing.T) {
		empty := make([]*drand.Participant, 0)
		needle := &drand.Participant{
			Address: "test",
		}
		res := Without(empty, needle)
		assert.Nil(st, res)
		assert.NotContains(st, res, needle)
	})
	t.Run("emptied haystack", func(st *testing.T) {
		list := make([]*drand.Participant, 0)
		needle := &drand.Participant{
			Address: "test",
		}
		list = append(list, needle)
		res := Without(list, needle)
		assert.Nil(st, res)
		assert.NotContains(st, list, needle)
	})
	t.Run("two same items in haystack", func(st *testing.T) {
		list := make([]*drand.Participant, 0)
		needle := &drand.Participant{
			Address: "test",
		}
		list = append(list, needle,
			&drand.Participant{Address: "yolo"},
			needle)
		assert.Len(st, list, 3)
		res := Without(list, needle)
		assert.Len(st, res, 1)
		assert.NotContains(st, list, needle)
	})
	t.Run("normal usage needle in the beginning", func(st *testing.T) {
		list := make([]*drand.Participant, 0)
		needle := &drand.Participant{
			Address: "test",
		}
		list = append(list,
			needle,
			&drand.Participant{Address: "one"},
			&drand.Participant{Address: "two"},
			&drand.Participant{Address: "three"},
		)
		assert.Len(st, list, 4)
		res := Without(list, needle)
		assert.Len(st, res, 3)
		assert.NotContains(st, list, needle)
		// the underlying list got its items zeroized
		assert.Contains(st, list, (*drand.Participant)(nil))
		assert.Len(st, list, 4)
	})
	t.Run("normal usage needle in the end", func(st *testing.T) {
		list := make([]*drand.Participant, 0)
		needle := &drand.Participant{
			Address: "test",
		}
		list = append(list,
			&drand.Participant{Address: "one"},
			&drand.Participant{Address: "two"},
			needle,
		)
		assert.Len(st, list, 3)
		res := Without(list, needle)
		assert.Len(st, res, 2)
		assert.NotContains(st, list, needle)
		// the underlying list got its items zeroized
		assert.Contains(st, list, (*drand.Participant)(nil))
		assert.Len(st, list, 3)
	})
	t.Run("normal usage needle in the middle", func(st *testing.T) {
		list := make([]*drand.Participant, 0)
		needle := &drand.Participant{
			Address: "test",
		}
		list = append(list,
			&drand.Participant{Address: "one"},
			needle,
			&drand.Participant{Address: "two"},
		)
		assert.Len(st, list, 3)
		res := Without(list, needle)
		assert.Len(st, res, 2)
		assert.NotContains(st, list, needle)
		// the underlying list got its items zeroized
		assert.Contains(st, list, (*drand.Participant)(nil))
		assert.Len(st, list, 3)
	})
	t.Run("nil needle with nil entries", func(st *testing.T) {
		list := make([]*drand.Participant, 3)
		list = append(list, &drand.Participant{
			Address: "one",
		})
		list = append(list, &drand.Participant{
			Address: "two",
		})
		assert.Contains(st, list, (*drand.Participant)(nil))
		assert.Len(st, list, 5)
		res := Without(list, nil)
		assert.Len(st, res, 2)
		assert.NotContains(st, list, nil)
		assert.NotContains(st, res, (*drand.Participant)(nil))
	})
	t.Run("nil needle", func(st *testing.T) {
		list := make([]*drand.Participant, 0)
		list = append(list, &drand.Participant{
			Address: "one",
		})
		list = append(list, &drand.Participant{
			Address: "two",
		})
		assert.Len(st, list, 2)
		res := Without(list, nil)
		assert.Len(st, res, 2)
		assert.NotContains(st, list, nil)
		assert.NotContains(st, list, (*drand.Participant)(nil))
	})
	t.Run("buggy needle", func(st *testing.T) {
		list := make([]*drand.Participant, 0)
		needle := &drand.Participant{
			Key: []byte("lacking an address"),
		}
		list = append(list, &drand.Participant{
			Address: "one",
		})
		list = append(list, &drand.Participant{
			Address: "two",
		})
		list = append(list, needle)
		assert.Len(st, list, 3)
		res := Without(list, needle)
		assert.Len(st, res, 2)
		assert.NotContains(st, list, needle)
	})
	t.Run("same address, different keys", func(st *testing.T) {
		list := make([]*drand.Participant, 0)
		needle := &drand.Participant{
			Address: "one",
			Key:     []byte("an address"),
		}
		list = append(list, &drand.Participant{
			Address: "one",
			Key:     []byte("another different address"),
		})
		list = append(list, &drand.Participant{
			Address: "two",
		})
		list = append(list, needle)
		assert.Len(st, list, 3)
		res := Without(list, needle)
		assert.Len(st, res, 2)
		assert.NotContains(st, list, needle)
		assert.NotContains(st, res, needle)
	})
}
