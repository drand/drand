package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeysSaveLoad(t *testing.T) {
	ps, _ := BatchIdentities(3)
	p := ps[0]
	fmt.Println(pwd())
	path := defaultPrivateFile()
	defer func() {
		//os.Remove(path)
	}()
	require.Nil(t, p.Save(path))

	p2 := new(Private)
	require.Nil(t, p2.Load(path))
	require.Equal(t, p.Key.String(), p2.Key.String())
	require.True(t, p.Public.Equal(p2.Public))
}
