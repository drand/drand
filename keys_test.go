package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeysSaveLoad(t *testing.T) {
	ps, group := BatchIdentities(3)
	p := ps[0]
	path := defaultPrivateFile()
	defer func() {
		os.Remove(path)
		os.Remove(publicFile(path))
	}()
	require.Nil(t, p.Save(path))

	p2 := new(Private)
	require.Nil(t, p2.Load(path))
	require.Equal(t, p.Key.String(), p2.Key.String())
	require.True(t, p.Public.Equal(p2.Public))

	groupPath := defaultGroupFile()
	defer func() {
		os.Remove(groupPath)
	}()
	require.Nil(t, group.Save(groupPath))
	g2 := new(Group)
	require.Nil(t, g2.Load(groupPath))

	require.Equal(t, group.Threshold, g2.Threshold)
	for i, p := range group.List {
		require.True(t, p.Equal(g2.List[i].Public))
	}
}

func TestKeysGroupPoint(t *testing.T) {
	n := 5
	_, group := BatchIdentities(n)
	points := group.Points()
	for i, p := range points {
		k := group.Public(i).Key
		require.Equal(t, p.String(), k.String())
	}
}
