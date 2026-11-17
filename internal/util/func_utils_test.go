package util

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcat(t *testing.T) {
	t.Run("no args returns nil", func(t *testing.T) {
		out := Concat[int]()
		assert.Nil(t, out)
	})
	t.Run("single slice", func(t *testing.T) {
		out := Concat([]int{1, 2, 3})
		assert.Equal(t, []int{1, 2, 3}, out)
	})
	t.Run("multiple slices in order", func(t *testing.T) {
		out := Concat([]int{1}, []int{2, 3}, []int{4})
		assert.Equal(t, []int{1, 2, 3, 4}, out)
	})
	t.Run("empty slices are skipped", func(t *testing.T) {
		out := Concat([]string{}, []string{"a"}, nil, []string{"b"})
		assert.Equal(t, []string{"a", "b"}, out)
	})
}

func TestCont(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		assert.True(t, Cont([]int{1, 2, 3}, 2))
	})
	t.Run("absent", func(t *testing.T) {
		assert.False(t, Cont([]int{1, 2, 3}, 4))
	})
	t.Run("empty haystack", func(t *testing.T) {
		assert.False(t, Cont([]int{}, 1))
	})
	t.Run("nil haystack", func(t *testing.T) {
		assert.False(t, Cont[string](nil, "x"))
	})
}

func TestFirst(t *testing.T) {
	t.Run("found returns pointer to matching value", func(t *testing.T) {
		got, err := First([]int{1, 2, 3, 4}, func(v int) bool { return v%2 == 0 })
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, 2, *got)
	})
	t.Run("not found returns error", func(t *testing.T) {
		got, err := First([]int{1, 3, 5}, func(v int) bool { return v%2 == 0 })
		require.Error(t, err)
		assert.Nil(t, got)
	})
	t.Run("empty slice returns error", func(t *testing.T) {
		got, err := First([]int{}, func(int) bool { return true })
		require.Error(t, err)
		assert.Nil(t, got)
	})
}

func TestFilter(t *testing.T) {
	t.Run("keeps matching", func(t *testing.T) {
		out := Filter([]int{1, 2, 3, 4, 5}, func(v int) bool { return v > 2 })
		assert.Equal(t, []int{3, 4, 5}, out)
	})
	t.Run("nothing matches returns nil", func(t *testing.T) {
		out := Filter([]int{1, 2}, func(int) bool { return false })
		assert.Nil(t, out)
	})
	t.Run("all match", func(t *testing.T) {
		out := Filter([]int{1, 2}, func(int) bool { return true })
		assert.Equal(t, []int{1, 2}, out)
	})
}

func TestErrorContains(t *testing.T) {
	err := errors.New("the quick brown fox")
	assert.True(t, ErrorContains(err, "brown"))
	assert.False(t, ErrorContains(err, "lazy"))
}
