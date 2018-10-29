package net

import (
	"context"
	"fmt"
	"net"

	"github.com/dedis/drand/protobuf/control"
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

func (g *ControlListener) Start() {
	if err := g.conns.Serve(g.lis); err != nil {
		slog.Fatalf("failed to serve: %s", err)
	}
}

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

func (c *ControlClient) Ping() error {
	_, err := c.client.PingPong(context.Background(), &control.Ping{})
	return err
}

// InitReshare sets up the node to be ready for a resharing protocol.
// oldPath and newPath represents the paths in the filesystems of the old group
// and the new group respectively. Leader is true if the destination node should
// start the protocol.
// NOTE: only group referral via filesystem path is supported at the moment.
// XXX Might be best to move to core/
func (c *ControlClient) InitReshare(oldPath, newPath string, leader bool) (*control.ReshareResponse, error) {
	request := &control.ReshareRequest{
		Old: &control.GroupInfo{
			Location: &control.GroupInfo_Path{oldPath},
		},
		New: &control.GroupInfo{
			Location: &control.GroupInfo_Path{newPath},
		},
		IsLeader: leader,
	}
	return c.client.InitReshare(context.Background(), request)
}

// InitDKG sets up the node to be ready for a first DKG protocol.
// groupPart
// NOTE: only group referral via filesystem path is supported at the moment.
// XXX Might be best to move to core/
func (c *ControlClient) InitDKG(groupPath string, leader bool) (*control.DKGResponse, error) {
	request := &control.DKGRequest{
		DkgGroup: &control.GroupInfo{
			Location: &control.GroupInfo_Path{groupPath},
		},
		IsLeader: leader,
	}
	return c.client.InitDKG(context.Background(), request)

}

func (c ControlClient) Share() (*control.ShareResponse, error) {
	return c.client.Share(context.Background(), &control.ShareRequest{})
}
func (c ControlClient) PublicKey() (*control.PublicKeyResponse, error) {
	return c.client.PublicKey(context.Background(), &control.PublicKeyRequest{})
}
func (c ControlClient) PrivateKey() (*control.PrivateKeyResponse, error) {
	return c.client.PrivateKey(context.Background(), &control.PrivateKeyRequest{})
}
func (c ControlClient) CollectiveKey() (*control.CokeyResponse, error) {
	return c.client.CollectiveKey(context.Background(), &control.CokeyRequest{})
}
func (c *ControlClient) Group() (*control.GroupResponse, error) {
	return c.client.Group(context.Background(), &control.GroupRequest{})
}

func controlListenAddr(port string) string {
	return fmt.Sprintf("%s:%s", "localhost", port)
}

//DefaultControlServer implements the functionalities of Control Service, and just as Default Service, it is used for testing.
type DefaultControlServer struct {
	C control.ControlServer
}

func (s *DefaultControlServer) PingPong(c context.Context, in *control.Ping) (*control.Pong, error) {
	return &control.Pong{}, nil
}

func (s *DefaultControlServer) Share(c context.Context, in *control.ShareRequest) (*control.ShareResponse, error) {
	if s.C == nil {
		return &control.ShareResponse{}, nil
	} else {
		return s.C.Share(c, in)
	}
}
func (s *DefaultControlServer) PublicKey(c context.Context, in *control.PublicKeyRequest) (*control.PublicKeyResponse, error) {
	if s.C == nil {
		return &control.PublicKeyResponse{}, nil
	} else {
		return s.C.PublicKey(c, in)
	}
}
func (s *DefaultControlServer) PrivateKey(c context.Context, in *control.PrivateKeyRequest) (*control.PrivateKeyResponse, error) {
	if s.C == nil {
		return &control.PrivateKeyResponse{}, nil
	} else {
		return s.C.PrivateKey(c, in)
	}
}
func (s *DefaultControlServer) CollectiveKey(c context.Context, in *control.CokeyRequest) (*control.CokeyResponse, error) {
	if s.C == nil {
		return &control.CokeyResponse{}, nil
	} else {
		return s.C.CollectiveKey(c, in)
	}
}

func (s *DefaultControlServer) Group(c context.Context, in *control.GroupRequest) (*control.GroupResponse, error) {
	if s.C == nil {
		return &control.GroupResponse{}, nil
	} else {
		return s.C.Group(c, in)
	}
}
