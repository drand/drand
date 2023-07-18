package net

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	common2 "github.com/drand/drand/common"
	"github.com/drand/drand/common/log"
	protoCommon "github.com/drand/drand/protobuf/common"
	control "github.com/drand/drand/protobuf/drand"
)

const grpcDefaultIPNetwork = "tcp"

// ControlListener is used to keep state for the connections of our drand instance
type ControlListener struct {
	log   log.Logger
	conns *grpc.Server
	lis   net.Listener
}

// NewGRPCListener registers the pairing between a ControlServer and a grpc server
func NewGRPCListener(l log.Logger, s Service, controlAddr string) (ControlListener, error) {
	grpcServer := grpc.NewServer()
	lis, err := newListener(controlAddr)
	if err != nil {
		l.Errorw("", "grpc listener", "failure", "err", err)
		return ControlListener{}, err
	}

	control.RegisterControlServer(grpcServer, s)
	control.RegisterDKGControlServer(grpcServer, s)

	return ControlListener{log: l, conns: grpcServer, lis: lis}, nil
}

// NewListener creates a net.Listener which should be shared between different gRPC servers
func newListener(controlAddr string) (net.Listener, error) {
	return net.Listen(listenAddrFor(controlAddr))
}

// Start the listener for the control commands
func (g *ControlListener) Start() {
	if err := g.conns.Serve(g.lis); err != nil {
		g.log.Errorw("", "control listener", "serve ended", "err", err)
	}
}

// Stop the listener and connections
// By default, the Stop call will try to terminate all connections nicely.
// However, after a timeout, it will forcefully close all connections and terminate.
func (g *ControlListener) Stop() {
	stopped := make(chan struct{})
	go func() {
		g.conns.GracefulStop()
		stopped <- struct{}{}
	}()
	select {
	case <-stopped:
	//nolint:gomnd // We want to forcefully terminate this in 5 seconds.
	case <-time.After(5 * time.Second):
		g.conns.Stop()
	}

	g.lis.Close()
}

// ControlClient is a struct that implement control.ControlClient and is used to
// request a Share to a ControlListener on a specific port
type ControlClient struct {
	log     log.Logger
	conn    *grpc.ClientConn
	client  control.ControlClient
	version common2.Version
}

// NewControlClient creates a client capable of issuing control commands to a
// 127.0.0.1 running drand node.
func NewControlClient(l log.Logger, addr string) (*ControlClient, error) {
	network, host := listenAddrFor(addr)
	if network != grpcDefaultIPNetwork {
		host = fmt.Sprintf("%s://%s", network, host)
	}

	conn, err := grpc.Dial(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		l.Errorw("", "control client", "connect failure", "err", err)
		return nil, err
	}

	c := control.NewControlClient(conn)

	return &ControlClient{
		log:     l,
		conn:    conn,
		client:  c,
		version: common2.GetAppVersion(),
	}, nil
}

func (c *ControlClient) Close() error {
	if c == nil || c.log == nil || c.conn == nil {
		return nil
	}
	c.log.Debugw("Closing connection for ControlClient")
	return c.conn.Close()
}

func (c *ControlClient) RemoteStatus(ct context.Context,
	addresses []*control.Address,
	beaconID string) (map[string]*control.StatusResponse, error) {
	metadata := protoCommon.Metadata{
		NodeVersion: c.version.ToProto(), BeaconID: beaconID,
	}

	packet := control.RemoteStatusRequest{
		Metadata:  &metadata,
		Addresses: addresses,
	}

	resp, err := c.client.RemoteStatus(ct, &packet)
	if err != nil {
		return nil, err
	}
	return resp.GetStatuses(), nil
}

// Ping the drand daemon to check if it's up and running
func (c *ControlClient) Ping() error {
	metadata := protoCommon.NewMetadata(c.version.ToProto())

	_, err := c.client.PingPong(context.Background(), &control.Ping{Metadata: metadata})
	return err
}

// LoadBeacon loads the beacon details
func (c *ControlClient) LoadBeacon(beaconID string) (*control.LoadBeaconResponse, error) {
	metadata := protoCommon.Metadata{
		NodeVersion: c.version.ToProto(), BeaconID: beaconID,
	}

	return c.client.LoadBeacon(context.Background(), &control.LoadBeaconRequest{Metadata: &metadata})
}

// ListBeaconIDs returns a list of all beacon ids
func (c *ControlClient) ListBeaconIDs() (*control.ListBeaconIDsResponse, error) {
	metadata := protoCommon.Metadata{
		NodeVersion: c.version.ToProto(),
	}

	return c.client.ListBeaconIDs(context.Background(), &control.ListBeaconIDsRequest{Metadata: &metadata})
}

// Status gets the current daemon status
func (c *ControlClient) Status(beaconID string) (*control.StatusResponse, error) {
	metadata := protoCommon.Metadata{
		NodeVersion: c.version.ToProto(), BeaconID: beaconID,
	}

	return c.client.Status(context.Background(), &control.StatusRequest{Metadata: &metadata})
}

// ListSchemes responds with the list of ids for the available schemes
func (c *ControlClient) ListSchemes() (*control.ListSchemesResponse, error) {
	metadata := protoCommon.NewMetadata(c.version.ToProto())

	return c.client.ListSchemes(context.Background(), &control.ListSchemesRequest{Metadata: metadata})
}

// PublicKey returns the public key of the remote node
func (c *ControlClient) PublicKey(beaconID string) (*control.PublicKeyResponse, error) {
	metadata := protoCommon.Metadata{NodeVersion: c.version.ToProto(), BeaconID: beaconID}

	return c.client.PublicKey(context.Background(), &control.PublicKeyRequest{Metadata: &metadata})
}

// PrivateKey returns the private key of the remote node
func (c *ControlClient) PrivateKey(beaconID string) (*control.PrivateKeyResponse, error) {
	metadata := protoCommon.Metadata{NodeVersion: c.version.ToProto(), BeaconID: beaconID}

	return c.client.PrivateKey(context.Background(), &control.PrivateKeyRequest{Metadata: &metadata})
}

// ChainInfo returns the collective key of the remote node
func (c *ControlClient) ChainInfo(beaconID string) (*control.ChainInfoPacket, error) {
	metadata := protoCommon.Metadata{NodeVersion: c.version.ToProto(), BeaconID: beaconID}

	return c.client.ChainInfo(context.Background(), &control.ChainInfoRequest{Metadata: &metadata})
}

// GroupFile returns the group file that the drand instance uses at the current
// time
func (c *ControlClient) GroupFile(beaconID string) (*control.GroupPacket, error) {
	metadata := protoCommon.Metadata{NodeVersion: c.version.ToProto(), BeaconID: beaconID}

	return c.client.GroupFile(context.Background(), &control.GroupRequest{Metadata: &metadata})
}

// Shutdown stops the daemon
func (c *ControlClient) Shutdown(beaconID string) (*control.ShutdownResponse, error) {
	metadata := protoCommon.Metadata{NodeVersion: c.version.ToProto(), BeaconID: beaconID}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return c.client.Shutdown(ctx, &control.ShutdownRequest{Metadata: &metadata})
}

const progressSyncQueue = 100

// StartCheckChain initiates the check chain process
func (c *ControlClient) StartCheckChain(cc context.Context, hashStr string, nodes []string, tls bool,
	upTo uint64, beaconID string) (outCh chan *control.SyncProgress, errCh chan error, e error) {
	// we need to make sure the beaconID is set in the metadata
	metadata := protoCommon.NewMetadata(c.version.ToProto())
	if beaconID == "" {
		metadata.BeaconID = common2.DefaultBeaconID
	} else {
		metadata.BeaconID = beaconID
	}

	hash, err := hex.DecodeString(hashStr)
	if err != nil {
		return nil, nil, err
	}

	if hashStr != common2.DefaultChainHash && hashStr != "" {
		metadata.ChainHash = hash
	}

	c.log.Infow("Launching a check request", "tls", tls, "upTo", upTo, "hash", hash, "beaconID", beaconID)

	if upTo == 0 {
		return nil, nil, fmt.Errorf("upTo must be greater than 0")
	}

	c.log.Infow("Starting to check chain consistency", "chain-hash", hash, "up to", upTo, "beaconID", beaconID)

	stream, err := c.client.StartCheckChain(cc, &control.StartSyncRequest{
		Nodes:    nodes,
		IsTls:    tls,
		UpTo:     upTo,
		Metadata: metadata,
	})

	if err != nil {
		c.log.Errorw("Error while checking chain consistency", "err", err)
		return nil, nil, err
	}

	outCh = make(chan *control.SyncProgress, progressSyncQueue)
	errCh = make(chan error)
	go func() {
		defer func() {
			close(outCh)
			close(errCh)
		}()

		for {
			resp, err := stream.Recv()
			if err != nil {
				errCh <- err
				return
			}

			select {
			case outCh <- resp:
			case <-cc.Done():
				return
			}
		}
	}()

	return outCh, errCh, nil
}

// StartFollowChain initiates the client catching up on an existing chain it is not part of
func (c *ControlClient) StartFollowChain(cc context.Context,
	hashStr string,
	nodes []string,
	tls bool,
	upTo uint64,
	beaconID string) (outCh chan *control.SyncProgress,
	errCh chan error, e error) {
	// we need to make sure the beaconID is set and also the chain hash to check integrity of the chain info
	metadata := protoCommon.NewMetadata(c.version.ToProto())
	if beaconID == "" {
		metadata.BeaconID = common2.DefaultBeaconID
	} else {
		metadata.BeaconID = beaconID
	}
	if hashStr == common2.DefaultChainHash || hashStr == "" {
		return nil, nil, fmt.Errorf("chain hash is not set properly, you cannot use the 'default' chain hash" +
			" to validate the integrity of the chain info when following a chain")
	}
	hash, err := hex.DecodeString(hashStr)
	if err != nil {
		return nil, nil, err
	}
	metadata.ChainHash = hash
	c.log.Infow("Launching a follow request", "nodes", nodes, "tls", tls, "upTo", upTo, "hash", hashStr, "beaconID", beaconID)
	stream, err := c.client.StartFollowChain(cc, &control.StartSyncRequest{
		Nodes:    nodes,
		IsTls:    tls,
		UpTo:     upTo,
		Metadata: metadata,
	})
	if err != nil {
		c.log.Errorw("Error while following chain", "err", err)
		return nil, nil, err
	}
	outCh = make(chan *control.SyncProgress, progressSyncQueue)
	// TODO: currently if the remote node terminates during the follow, it won't close the client side process
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
	_, err := c.client.BackupDatabase(context.Background(), &control.BackupDBRequest{OutputFile: outFile, Metadata: &metadata})
	return err
}

// DefaultControlServer implements the functionalities of Control Service, and just as Default Service, it is used for testing.
type DefaultControlServer struct {
	C control.ControlServer
}

// PingPong sends a ping to the server
func (s *DefaultControlServer) PingPong(_ context.Context, _ *control.Ping) (*control.Pong, error) {
	return &control.Pong{}, nil
}

// Status initiates a status request
func (s *DefaultControlServer) Status(c context.Context, in *control.StatusRequest) (*control.StatusResponse, error) {
	if s.C == nil {
		return &control.StatusResponse{}, nil
	}
	return s.C.Status(c, in)
}

func (s *DefaultControlServer) RemoteStatus(c context.Context, in *control.RemoteStatusRequest) (*control.RemoteStatusResponse, error) {
	if s.C == nil {
		return &control.RemoteStatusResponse{}, nil
	}
	return s.C.RemoteStatus(c, in)
}

// PublicKey gets the node's public key
func (s *DefaultControlServer) PublicKey(c context.Context, in *control.PublicKeyRequest) (*control.PublicKeyResponse, error) {
	if s.C == nil {
		return &control.PublicKeyResponse{}, nil
	}
	return s.C.PublicKey(c, in)
}

// ChainInfo gets the current chain information from the ndoe
func (s *DefaultControlServer) ChainInfo(c context.Context, in *control.ChainInfoRequest) (*control.ChainInfoPacket, error) {
	if s.C == nil {
		return &control.ChainInfoPacket{}, nil
	}
	return s.C.ChainInfo(c, in)
}
