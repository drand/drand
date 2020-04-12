package net

import (
	"context"
	"fmt"
	"net"
	"time"

	control "github.com/drand/drand/protobuf/drand"

	"github.com/nikkolasg/slog"
	"google.golang.org/grpc"
)

//ControlListener is used to keep state of the connections of our drand instance
type ControlListener struct {
	conns *grpc.Server
	lis   net.Listener
}

//NewTCPGrpcControlListener registers the pairing between a ControlServer and a grpx server
func NewTCPGrpcControlListener(s control.ControlServer, port string) ControlListener {
	lis, err := net.Listen("tcp", controlListenAddr(port))
	if err != nil {
		slog.Fatalf("grpc listener failure: %s", err)
		return ControlListener{}
	}
	grpcServer := grpc.NewServer()
	control.RegisterControlServer(grpcServer, s)
	return ControlListener{conns: grpcServer, lis: lis}
}

// Start the listener for the control commands
func (g *ControlListener) Start() {
	if err := g.conns.Serve(g.lis); err != nil {
		slog.Fatalf("failed to serve: %s", err)
	}
}

// Stop the listener and connections
func (g *ControlListener) Stop() {
	g.conns.Stop()
}

//ControlClient is a struct that implement control.ControlClient and is used to
//request a Share to a ControlListener on a specific port
type ControlClient struct {
	conn   *grpc.ClientConn
	client control.ControlClient
}

// NewControlClient creates a client capable of issuing control commands to a
// localhost running drand node.
func NewControlClient(port string) (*ControlClient, error) {
	var conn *grpc.ClientConn
	conn, err := grpc.Dial(controlListenAddr(port), grpc.WithInsecure())
	if err != nil {
		slog.Fatalf("control: did not connect: %s", err)
		return nil, err
	}
	c := control.NewControlClient(conn)
	return &ControlClient{conn: conn, client: c}, nil
}

// Ping the drand daemon to check if it's up and running
func (c *ControlClient) Ping() error {
	_, err := c.client.PingPong(context.Background(), &control.Ping{})
	return err
}

// InitReshare sets up the node to be ready for a resharing protocol.
// NOTE: only group referral via filesystem path is supported at the moment.
// XXX Might be best to move to core/
func (c *ControlClient) InitReshareLeader(nodes, threshold int, timeout string, secret string, oldPath string, offset int) (*control.GroupPacket, error) {
	request := &control.InitResharePacket{
		Old: &control.GroupInfo{
			Location: &control.GroupInfo_Path{Path: oldPath},
		},
		Info: &control.SetupInfoPacket{
			Nodes:        uint32(nodes),
			Threshold:    uint32(threshold),
			Leader:       true,
			Timeout:      timeout,
			Secret:       secret,
			BeaconOffset: uint32(offset),
		},
	}
	return c.client.InitReshare(context.Background(), request)
}

func (c *ControlClient) InitReshare(leader Peer, nodes, threshold int, timeout string, secret string, oldPath string) (*control.GroupPacket, error) {
	request := &control.InitResharePacket{
		Old: &control.GroupInfo{
			Location: &control.GroupInfo_Path{Path: oldPath},
		},
		Info: &control.SetupInfoPacket{
			Nodes:         uint32(nodes),
			Threshold:     uint32(threshold),
			Leader:        false,
			LeaderAddress: leader.Address(),
			LeaderTls:     leader.IsTLS(),
			Timeout:       timeout,
			Secret:        secret,
		},
	}
	return c.client.InitReshare(context.Background(), request)
}

// InitDKG sets up the node to be ready for a first DKG protocol.
// groupPart
// NOTE: only group referral via filesystem path is supported at the moment.
// XXX Might be best to move to core/
func (c *ControlClient) InitDKGLeader(nodes, threshold int, beaconPeriod time.Duration, timeout string, entropy *control.EntropyInfo, secret string, offset int) (*control.GroupPacket, error) {
	request := &control.InitDKGPacket{
		Info: &control.SetupInfoPacket{
			Nodes:        uint32(nodes),
			Threshold:    uint32(threshold),
			Leader:       true,
			Timeout:      timeout,
			Secret:       secret,
			BeaconOffset: uint32(offset),
		},
		Entropy:      entropy,
		BeaconPeriod: uint32(beaconPeriod.Seconds()),
	}
	return c.client.InitDKG(context.Background(), request)
}

func (c *ControlClient) InitDKG(leader Peer, nodes, threshold int, timeout string, entropy *control.EntropyInfo, secret string) (*control.GroupPacket, error) {
	request := &control.InitDKGPacket{
		Info: &control.SetupInfoPacket{
			Nodes:         uint32(nodes),
			Threshold:     uint32(threshold),
			Leader:        false,
			LeaderAddress: leader.Address(),
			LeaderTls:     leader.IsTLS(),
			Timeout:       timeout,
			Secret:        secret,
		},
		Entropy: entropy,
	}
	return c.client.InitDKG(context.Background(), request)
}

// Share returns the share of the remote node
func (c ControlClient) Share() (*control.ShareResponse, error) {
	return c.client.Share(context.Background(), &control.ShareRequest{})
}

// PublicKey returns the public key of the remote node
func (c ControlClient) PublicKey() (*control.PublicKeyResponse, error) {
	return c.client.PublicKey(context.Background(), &control.PublicKeyRequest{})
}

// PrivateKey returns the private key of the remote node
func (c ControlClient) PrivateKey() (*control.PrivateKeyResponse, error) {
	return c.client.PrivateKey(context.Background(), &control.PrivateKeyRequest{})
}

// CollectiveKey returns the collective key of the remote node
func (c ControlClient) CollectiveKey() (*control.CokeyResponse, error) {
	return c.client.CollectiveKey(context.Background(), &control.CokeyRequest{})
}

// GroupFile returns the group file that the drand instance uses at the current
// time
func (c ControlClient) GroupFile() (*control.GroupPacket, error) {
	return c.client.GroupFile(context.Background(), &control.GroupRequest{})
}

// Shutdown stops the daemon
func (c ControlClient) Shutdown() (*control.ShutdownResponse, error) {
	return c.client.Shutdown(context.Background(), &control.ShutdownRequest{})
}

func controlListenAddr(port string) string {
	return fmt.Sprintf("%s:%s", "localhost", port)
}

//DefaultControlServer implements the functionalities of Control Service, and just as Default Service, it is used for testing.
type DefaultControlServer struct {
	C control.ControlServer
}

// PingPong ...
func (s *DefaultControlServer) PingPong(c context.Context, in *control.Ping) (*control.Pong, error) {
	return &control.Pong{}, nil
}

// Share ...
func (s *DefaultControlServer) Share(c context.Context, in *control.ShareRequest) (*control.ShareResponse, error) {
	if s.C == nil {
		return &control.ShareResponse{}, nil
	}
	return s.C.Share(c, in)
}

// PublicKey ...
func (s *DefaultControlServer) PublicKey(c context.Context, in *control.PublicKeyRequest) (*control.PublicKeyResponse, error) {
	if s.C == nil {
		return &control.PublicKeyResponse{}, nil
	}
	return s.C.PublicKey(c, in)
}

// PrivateKey ...
func (s *DefaultControlServer) PrivateKey(c context.Context, in *control.PrivateKeyRequest) (*control.PrivateKeyResponse, error) {
	if s.C == nil {
		return &control.PrivateKeyResponse{}, nil
	}
	return s.C.PrivateKey(c, in)
}

// CollectiveKey ...
func (s *DefaultControlServer) CollectiveKey(c context.Context, in *control.CokeyRequest) (*control.CokeyResponse, error) {
	if s.C == nil {
		return &control.CokeyResponse{}, nil
	}
	return s.C.CollectiveKey(c, in)
}
