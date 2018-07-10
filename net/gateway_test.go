package net

import (
	"context"
	"fmt"
	"net"
	"os"
	"path"
	run "runtime"
	"testing"
	"time"

	"github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/protobuf/drand"
	"github.com/kabukky/httpscerts"
	"github.com/stretchr/testify/require"
)

type testPeer struct {
	addr string
	t    bool
}

func (t *testPeer) Address() string {
	return t.addr
}

func (t *testPeer) IsTLS() bool {
	return t.t
}

type testService struct {
	round uint64
}

func (t *testService) Public(context.Context, *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	return &drand.PublicRandResponse{Round: t.round}, nil
}

func (t *testService) Private(context.Context, *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	return &drand.PrivateRandResponse{}, nil
}
func (t *testService) Setup(c context.Context, in *dkg.DKGPacket) (*dkg.DKGResponse, error) {
	return &dkg.DKGResponse{}, nil
}
func (t *testService) NewBeacon(c context.Context, in *drand.BeaconRequest) (*drand.BeaconResponse, error) {
	return &drand.BeaconResponse{}, nil
}

func TestListener(t *testing.T) {
	addr1 := "127.0.0.1:4000"
	peer1 := &testPeer{addr1, false}
	//addr2 := "127.0.0.1:4001"
	service1 := &testService{42}

	lis1 := NewTCPGrpcListener(addr1, service1)
	go lis1.Start()
	defer lis1.Stop()
	time.Sleep(100 * time.Millisecond)

	client := NewGrpcClient()
	resp, err := client.Public(peer1, &drand.PublicRandRequest{})
	require.Nil(t, err)
	expected := &drand.PublicRandResponse{Round: service1.round}
	require.Equal(t, expected.GetRound(), resp.GetRound())

	rest := NewRestClient()
	resp, err = rest.Public(peer1, &drand.PublicRandRequest{})
	require.NoError(t, err)
	expected = &drand.PublicRandResponse{Round: service1.round}
	require.Equal(t, expected.GetRound(), resp.GetRound())
}

// ref https://bbengfort.github.io/programmer/2017/03/03/secure-grpc.html
func TestListenerTLS(t *testing.T) {
	if run.GOOS == "windows" {
		fmt.Println("Skipping TestClientTLS as operating on Windows")
		t.Skip("crypto/x509: system root pool is not available on Windows")
	} else {
		addr1 := "127.0.0.1:4000"
		peer1 := &testPeer{addr1, true}

		tmpDir := path.Join(os.TempDir(), "drand-net")
		require.NoError(t, os.MkdirAll(tmpDir, 0766))
		defer os.RemoveAll(tmpDir)
		certPath := path.Join(tmpDir, "server.crt")
		keyPath := path.Join(tmpDir, "server.key")
		if httpscerts.Check(certPath, keyPath) != nil {
			h, _, _ := net.SplitHostPort(addr1)
			require.NoError(t, httpscerts.Generate(certPath, keyPath, h))
			//require.NoError(t, httpscerts.Generate(certPath, keyPath, addr1))
		}

		service1 := &testService{42}

		lis1, err := NewTLSGrpcListener(addr1, certPath, keyPath, service1)
		require.NoError(t, err)
		go lis1.Start()
		defer lis1.Stop()
		time.Sleep(100 * time.Millisecond)

		require.Equal(t, peer1.Address(), addr1)
		certManager := NewCertManager()
		certManager.Add(certPath)

		client := NewGrpcClientFromCertManager(certManager)
		resp, err := client.Public(peer1, &drand.PublicRandRequest{})
		require.Nil(t, err)
		expected := &drand.PublicRandResponse{Round: service1.round}
		require.Equal(t, expected.GetRound(), resp.GetRound())

		rest := NewRestClientFromCertManager(certManager)
		resp, err = rest.Public(peer1, &drand.PublicRandRequest{})
		require.NoError(t, err)
		expected = &drand.PublicRandResponse{Round: service1.round}
		require.Equal(t, expected.GetRound(), resp.GetRound())
	}
}
