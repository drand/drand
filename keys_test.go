package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeysPrivateLoad(t *testing.T) {
	ps, _ := BatchIdentities(1)
	p := ps[0]

	path := defaultPrivateFile()
	defer func() {
		os.Remove(path)
	}()
	require.Nil(t, p.Save(path))

	p2 := new(Private)
	require.Nil(t, p2.Load(path))
	require.Equal(t, p.Key.String(), p2.Key.String())
}
