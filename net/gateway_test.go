package net

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path"
	run "runtime"
	"testing"
	"time"

	"github.com/drand/drand/protobuf/drand"
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

type testRandomnessServer struct {
	*EmptyServer
	round uint64
}

func (t *testRandomnessServer) NewBeacon(context.Context, *drand.BeaconPacket) (*drand.Empty, error) {
	return new(drand.Empty), errors.New("no beacon")
}
func (t *testRandomnessServer) PublicRand(context.Context, *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	return &drand.PublicRandResponse{Round: t.round}, nil
}
func (t *testRandomnessServer) PrivateRand(context.Context, *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	return &drand.PrivateRandResponse{}, nil
}
func (t *testRandomnessServer) Group(context.Context, *drand.GroupRequest) (*drand.GroupPacket, error) {
	return nil, nil
}
func (t *testRandomnessServer) DistKey(context.Context, *drand.DistKeyRequest) (*drand.DistKeyResponse, error) {
	return nil, nil
}
func (t *testRandomnessServer) Home(context.Context, *drand.HomeRequest) (*drand.HomeResponse, error) {
	return nil, nil
}

func TestListener(t *testing.T) {
	ctx := context.Background()
	addr1 := "127.0.0.1:4000"
	peer1 := &testPeer{addr1, false}
	//addr2 := "127.0.0.1:4001"
	randServer := &testRandomnessServer{round: 42}

	lis1 := NewTCPGrpcListener(addr1, randServer)
	go lis1.Start()
	defer lis1.Stop()
	time.Sleep(100 * time.Millisecond)

	client := NewGrpcClient()
	resp, err := client.PublicRand(ctx, peer1, &drand.PublicRandRequest{})
	require.NoError(t, err)
	expected := &drand.PublicRandResponse{Round: randServer.round}
	require.Equal(t, expected.GetRound(), resp.GetRound())

	err = client.PartialBeacon(ctx, peer1, &drand.PartialBeaconPacket{})
	require.Error(t, err)

	rest := NewRestClient()
	resp, err = rest.PublicRand(ctx, peer1, &drand.PublicRandRequest{})
	require.NoError(t, err)
	expected = &drand.PublicRandResponse{Round: randServer.round}
	require.Equal(t, expected.GetRound(), resp.GetRound())
}

// ref https://bbengfort.github.io/programmer/2017/03/03/secure-grpc.html
func TestListenerTLS(t *testing.T) {
	if run.GOOS == "windows" {
		fmt.Println("Skipping TestClientTLS as operating on Windows")
		t.Skip("crypto/x509: system root pool is not available on Windows")
	}
	ctx := context.Background()
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

	randServer := &testRandomnessServer{round: 42}

	lis1, err := NewRESTListenerForPublicWithTLS(addr1, certPath, keyPath, randServer)
	require.NoError(t, err)
	go lis1.Start()
	defer lis1.Stop()
	time.Sleep(100 * time.Millisecond)

	require.Equal(t, peer1.Address(), addr1)
	certManager := NewCertManager()
	certManager.Add(certPath)

	client := NewGrpcClientFromCertManager(certManager)
	resp, err := client.PublicRand(ctx, peer1, &drand.PublicRandRequest{})
	require.Nil(t, err)
	expected := &drand.PublicRandResponse{Round: randServer.round}
	require.Equal(t, expected.GetRound(), resp.GetRound())

	rest := NewRestClientFromCertManager(certManager)
	resp, err = rest.PublicRand(ctx, peer1, &drand.PublicRandRequest{})
	require.NoError(t, err)
	expected = &drand.PublicRandResponse{Round: randServer.round}
	require.Equal(t, expected.GetRound(), resp.GetRound())
}
