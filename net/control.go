package net

import (
	ctx "context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/drand/drand/common"
	"github.com/drand/drand/log"
	protoCommon "github.com/drand/drand/protobuf/common"

	//nolint:stylecheck
	"github.com/drand/drand/protobuf/drand"

	//nolint:stylecheck
	control "github.com/drand/drand/protobuf/drand"

	"google.golang.org/grpc"
)

const grpcDefaultIPNetwork = "tcp"

// ControlListener is used to keep state of the connections of our drand instance
type ControlListener struct {
	conns *grpc.Server
	lis   net.Listener
}

// NewTCPGrpcControlListener registers the pairing between a ControlServer and a grpc server
func NewTCPGrpcControlListener(s control.ControlServer, controlAddr string) ControlListener {
	lis, err := net.Listen(controlListenAddr(controlAddr))
	if err != nil {
		log.DefaultLogger().Errorw("", "grpc listener", "failure", "err", err)
		return ControlListener{}
	}
	grpcServer := grpc.NewServer()
	control.RegisterControlServer(grpcServer, s)
	return ControlListener{conns: grpcServer, lis: lis}
}

// Start the listener for the control commands
func (g *ControlListener) Start() {
	if err := g.conns.Serve(g.lis); err != nil {
		log.DefaultLogger().Errorw("", "control listener", "serve ended", "err", err)
	}
}

// Stop the listener and connections
func (g *ControlListener) Stop() {
	g.conns.Stop()
}

// ControlClient is a struct that implement control.ControlClient and is used to
// request a Share to a ControlListener on a specific port
type ControlClient struct {
	conn    *grpc.ClientConn
	client  control.ControlClient
	version common.Version
}

// NewControlClient creates a client capable of issuing control commands to a
// localhost running drand node.
func NewControlClient(addr string) (*ControlClient, error) {
	var conn *grpc.ClientConn
	network, host := controlListenAddr(addr)
	if network != grpcDefaultIPNetwork {
		host = fmt.Sprintf("%s://%s", network, host)
	}

	conn, err := grpc.Dial(host, grpc.WithInsecure())
	if err != nil {
		log.DefaultLogger().Errorw("", "control client", "connect failure", "err", err)
		return nil, err
	}

	c := control.NewControlClient(conn)

	return &ControlClient{
		conn:    conn,
		client:  c,
		version: common.GetAppVersion(),
	}, nil
}

func (c *ControlClient) RemoteStatus(ct ctx.Context,
	addresses []*drand.Address,
	beaconID string) (map[string]*drand.StatusResponse, error) {
	metadata := protoCommon.Metadata{
		NodeVersion: c.version.ToProto(), BeaconID: beaconID,
	}

	packet := drand.RemoteStatusRequest{
		Metadata:  &metadata,
		Addresses: addresses,
	}

	resp, err := c.client.RemoteStatus(ct, &packet)
	return resp.GetStatuses(), err
}

// Ping the drand daemon to check if it's up and running
func (c *ControlClient) Ping() error {
	metadata := protoCommon.NewMetadata(c.version.ToProto())

	_, err := c.client.PingPong(ctx.Background(), &control.Ping{Metadata: metadata})
	return err
}

// Resurrect
func (c *ControlClient) ResurrectBeacon(beaconID string) (*control.ResurrectBeaconResponse, error) {
	metadata := protoCommon.Metadata{
		NodeVersion: c.version.ToProto(), BeaconID: beaconID,
	}

	resp, err := c.client.ResurrectBeacon(ctx.Background(), &control.ResurrectBeaconRequest{Metadata: &metadata})
	return resp, err
}

// Status gets the current daemon status
func (c *ControlClient) Status(beaconID string) (*control.StatusResponse, error) {
	metadata := protoCommon.Metadata{
		NodeVersion: c.version.ToProto(), BeaconID: beaconID,
	}

	resp, err := c.client.Status(ctx.Background(), &control.StatusRequest{Metadata: &metadata})
	return resp, err
}

// ListSchemes responds with the list of ids for the available schemes
func (c *ControlClient) ListSchemes() (*control.ListSchemesResponse, error) {
	metadata := protoCommon.NewMetadata(c.version.ToProto())

	resp, err := c.client.ListSchemes(ctx.Background(), &control.ListSchemesRequest{Metadata: metadata})
	return resp, err
}

// InitReshareLeader sets up the node to be ready for a resharing protocol.
// NOTE: only group referral via filesystem path is supported at the moment.
// XXX Might be best to move to core/
func (c *ControlClient) InitReshareLeader(
	nodes, threshold int,
	timeout, catchupPeriod time.Duration,
	secret, oldPath string,
	offset int,
	beaconID string) (*control.GroupPacket, error) {
	metadata := protoCommon.Metadata{NodeVersion: c.version.ToProto(), BeaconID: beaconID}

	request := &control.InitResharePacket{
		Old: &control.GroupInfo{
			Location: &control.GroupInfo_Path{Path: oldPath},
		},
		Info: &control.SetupInfoPacket{
			Nodes:        uint32(nodes),
			Threshold:    uint32(threshold),
			Leader:       true,
			Timeout:      uint32(timeout.Seconds()),
			Secret:       []byte(secret),
			BeaconOffset: uint32(offset),
			Metadata:     &metadata,
		},
		CatchupPeriodChanged: catchupPeriod >= 0,
		CatchupPeriod:        uint32(catchupPeriod.Seconds()),
		Metadata:             &metadata,
	}

	return c.client.InitReshare(ctx.Background(), request)
}

// InitReshare sets up the node to be ready for a resharing protocol.
func (c *ControlClient) InitReshare(leader Peer, secret, oldPath string, force bool, beaconID string) (*control.GroupPacket, error) {
	metadata := protoCommon.Metadata{NodeVersion: c.version.ToProto(), BeaconID: beaconID}

	request := &control.InitResharePacket{
		Old: &control.GroupInfo{
			Location: &control.GroupInfo_Path{Path: oldPath},
		},
		Info: &control.SetupInfoPacket{
			Leader:        false,
			LeaderAddress: leader.Address(),
			LeaderTls:     leader.IsTLS(),
			Secret:        []byte(secret),
			Force:         force,
			Metadata:      &metadata,
		},
		Metadata: &metadata,
	}

	return c.client.InitReshare(ctx.Background(), request)
}

// InitDKGLeader sets up the node to be ready for a first DKG protocol.
// groupPart
// NOTE: only group referral via filesystem path is supported at the moment.
// XXX Might be best to move to core/
func (c *ControlClient) InitDKGLeader(
	nodes, threshold int,
	beaconPeriod, catchupPeriod, timeout time.Duration,
	entropy *control.EntropyInfo,
	secret string,
	offset int,
	schemeID string,
	beaconID string) (*control.GroupPacket, error) {
	metadata := protoCommon.Metadata{NodeVersion: c.version.ToProto(), BeaconID: beaconID}

	request := &control.InitDKGPacket{
		Info: &control.SetupInfoPacket{
			Nodes:        uint32(nodes),
			Threshold:    uint32(threshold),
			Leader:       true,
			Timeout:      uint32(timeout.Seconds()),
			Secret:       []byte(secret),
			BeaconOffset: uint32(offset),
			Metadata:     &metadata,
		},
		Entropy:       entropy,
		BeaconPeriod:  uint32(beaconPeriod.Seconds()),
		CatchupPeriod: uint32(catchupPeriod.Seconds()),
		SchemeID:      schemeID,
		Metadata:      &metadata,
	}

	return c.client.InitDKG(ctx.Background(), request)
}

// InitDKG sets up the node to be ready for a first DKG protocol.
func (c *ControlClient) InitDKG(leader Peer, entropy *control.EntropyInfo, secret, beaconID string) (*control.GroupPacket, error) {
	metadata := protoCommon.Metadata{NodeVersion: c.version.ToProto(), BeaconID: beaconID}

	request := &control.InitDKGPacket{
		Info: &control.SetupInfoPacket{
			Leader:        false,
			LeaderAddress: leader.Address(),
			LeaderTls:     leader.IsTLS(),
			Secret:        []byte(secret),
			Metadata:      &metadata,
		},
		Entropy:  entropy,
		Metadata: &metadata,
	}

	return c.client.InitDKG(ctx.Background(), request)
}

// Share returns the share of the remote node
func (c *ControlClient) Share(beaconID string) (*control.ShareResponse, error) {
	metadata := protoCommon.Metadata{NodeVersion: c.version.ToProto(), BeaconID: beaconID}

	return c.client.Share(ctx.Background(), &control.ShareRequest{Metadata: &metadata})
}

// PublicKey returns the public key of the remote node
func (c *ControlClient) PublicKey(beaconID string) (*control.PublicKeyResponse, error) {
	metadata := protoCommon.Metadata{NodeVersion: c.version.ToProto(), BeaconID: beaconID}

	return c.client.PublicKey(ctx.Background(), &control.PublicKeyRequest{Metadata: &metadata})
}

// PrivateKey returns the private key of the remote node
func (c *ControlClient) PrivateKey(beaconID string) (*control.PrivateKeyResponse, error) {
	metadata := protoCommon.Metadata{NodeVersion: c.version.ToProto(), BeaconID: beaconID}

	return c.client.PrivateKey(ctx.Background(), &control.PrivateKeyRequest{Metadata: &metadata})
}

// ChainInfo returns the collective key of the remote node
func (c *ControlClient) ChainInfo(beaconID string) (*control.ChainInfoPacket, error) {
	metadata := protoCommon.Metadata{NodeVersion: c.version.ToProto(), BeaconID: beaconID}

	return c.client.ChainInfo(ctx.Background(), &control.ChainInfoRequest{Metadata: &metadata})
}

// GroupFile returns the group file that the drand instance uses at the current
// time
func (c *ControlClient) GroupFile(beaconID string) (*control.GroupPacket, error) {
	metadata := protoCommon.Metadata{NodeVersion: c.version.ToProto(), BeaconID: beaconID}

	return c.client.GroupFile(ctx.Background(), &control.GroupRequest{Metadata: &metadata})
}

// Shutdown stops the daemon
func (c *ControlClient) Shutdown(beaconID string) (*control.ShutdownResponse, error) {
	metadata := protoCommon.Metadata{NodeVersion: c.version.ToProto(), BeaconID: beaconID}
	return c.client.Shutdown(ctx.Background(), &control.ShutdownRequest{Metadata: &metadata})
}

const progressFollowQueue = 100

// StartFollowChain initates the client catching up on an existing chain it is not part of
func (c *ControlClient) StartFollowChain(cc ctx.Context,
	hash string,
	nodes []string,
	tls bool,
	upTo uint64,
	beaconID string) (outCh chan *control.FollowProgress,
	errCh chan error, e error) {
	metadata := protoCommon.NewMetadata(c.version.ToProto())
	metadata.BeaconID = beaconID

	stream, err := c.client.StartFollowChain(cc, &control.StartFollowRequest{
		InfoHash: hash,
		Nodes:    nodes,
		IsTls:    tls,
		UpTo:     upTo,
		Metadata: metadata,
	})
	if err != nil {
		return nil, nil, err
	}
	outCh = make(chan *control.FollowProgress, progressFollowQueue)
	errCh = make(chan error, 1)
	go func() {
		for {
			resp, err := stream.Recv()
			if err != nil {
				errCh <- err
				close(errCh)
				close(outCh)
				return
			}
			select {
			case outCh <- resp:
			case <-cc.Done():
				close(errCh)
				close(outCh)
				return
			}
		}
	}()
	return outCh, errCh, nil
}

// BackupDB backs up the database to a file
func (c *ControlClient) BackupDB(outFile, beaconID string) error {
	metadata := protoCommon.Metadata{NodeVersion: c.version.ToProto(), BeaconID: beaconID}
	_, err := c.client.BackupDatabase(ctx.Background(), &control.BackupDBRequest{OutputFile: outFile, Metadata: &metadata})
	return err
}

// controlListenAddr parses the control address as specified, into a dialable / listenable address
func controlListenAddr(listenAddr string) (network, addr string) {
	if strings.HasPrefix(listenAddr, "unix://") {
		return "unix", strings.TrimPrefix(listenAddr, "unix://")
	}
	if strings.Contains(listenAddr, ":") {
		return grpcDefaultIPNetwork, listenAddr
	}
	return grpcDefaultIPNetwork, fmt.Sprintf("%s:%s", "localhost", listenAddr)
}

// DefaultControlServer implements the functionalities of Control Service, and just as Default Service, it is used for testing.
type DefaultControlServer struct {
	C control.ControlServer
}

// PingPong sends a ping to the server
func (s *DefaultControlServer) PingPong(c ctx.Context, in *control.Ping) (*control.Pong, error) {
	return &control.Pong{}, nil
}

// Status initiates a status request
func (s *DefaultControlServer) Status(c ctx.Context, in *control.StatusRequest) (*control.StatusResponse, error) {
	if s.C == nil {
		return &control.StatusResponse{}, nil
	}
	return s.C.Status(c, in)
}

func (s *DefaultControlServer) RemoteStatus(c ctx.Context, in *control.RemoteStatusRequest) (*control.RemoteStatusResponse, error) {
	if s.C == nil {
		return &control.RemoteStatusResponse{}, nil
	}
	return s.C.RemoteStatus(c, in)
}

// Share initiates a share request
func (s *DefaultControlServer) Share(c ctx.Context, in *control.ShareRequest) (*control.ShareResponse, error) {
	if s.C == nil {
		return &control.ShareResponse{}, nil
	}
	return s.C.Share(c, in)
}

// PublicKey gets the node's public key
func (s *DefaultControlServer) PublicKey(c ctx.Context, in *control.PublicKeyRequest) (*control.PublicKeyResponse, error) {
	if s.C == nil {
		return &control.PublicKeyResponse{}, nil
	}
	return s.C.PublicKey(c, in)
}

// PrivateKey gets the node's private key
func (s *DefaultControlServer) PrivateKey(c ctx.Context, in *control.PrivateKeyRequest) (*control.PrivateKeyResponse, error) {
	if s.C == nil {
		return &control.PrivateKeyResponse{}, nil
	}
	return s.C.PrivateKey(c, in)
}

// ChainInfo gets the current chain information from the ndoe
func (s *DefaultControlServer) ChainInfo(c ctx.Context, in *control.ChainInfoRequest) (*control.ChainInfoPacket, error) {
	if s.C == nil {
		return &control.ChainInfoPacket{}, nil
	}
	return s.C.ChainInfo(c, in)
}
