package api

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/dedis/drand/core"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/test"
	"github.com/stretchr/testify/require"
)

func TestClientPrivate(t *testing.T) {
	drands, pairs, dir := BatchNewDrand(5)
	defer CloseAllDrands(drands)
	defer os.RemoveAll(dir)

	client := NewGrpcClient()
	buff, err := client.Private(pairs[0].Public)
	require.Nil(t, err)
	require.NotNil(t, buff)
	require.Len(t, buff, 32)

	client = NewRESTClient()
	buff, err = client.Private(pairs[0].Public)
	require.Nil(t, err)
	require.NotNil(t, buff)
	require.Len(t, buff, 32)

}

func BatchNewDrand(n int, opts ...core.ConfigOption) ([]*core.Drand, []*key.Pair, string) {
	privs, group := test.BatchIdentities(n)
	var err error
	drands := make([]*core.Drand, n, n)
	tmp := os.TempDir()
	dir, err := ioutil.TempDir(tmp, "drand")
	if err != nil {
		panic(err)
	}
	pairs := make([]*key.Pair, n, n)
	for i := 0; i < n; i++ {
		s := test.NewKeyStore()
		s.SaveKeyPair(privs[i])
		pairs[i] = privs[i]
		// give each one their own private folder
		dbFolder := path.Join(dir, fmt.Sprintf("db-%d", i))
		drands[i], err = core.NewDrand(s, group, core.NewConfig(append([]core.ConfigOption{core.WithDbFolder(dbFolder)}, opts...)...))
		if err != nil {
			panic(err)
		}
	}
	return drands, pairs, dir
}

func CloseAllDrands(drands []*core.Drand) {
	for i := 0; i < len(drands); i++ {
		drands[i].Stop()
	}
}
