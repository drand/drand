package core

import (
	"context"
	"fmt"

	"github.com/drand/drand/key"

	"github.com/drand/drand/common/scheme"

	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
)

// InitDKG take a InitDKGPacket, extracts the informations needed and wait for
// the DKG protocol to finish. If the request specifies this node is a leader,
// it starts the DKG protocol.
func (dd *DrandDaemon) InitDKG(c context.Context, in *drand.InitDKGPacket) (*drand.GroupPacket, error) {
	bp, beaconID, err := dd.getBeaconProcess(in.GetMetadata())
	if err != nil {
		store, isStoreLoaded := dd.initialStores[beaconID]
		if !isStoreLoaded {
			dd.log.Infow("", "beacon_id", beaconID, "init_dkg", "loading store from disk")

			newStore := key.NewFileStore(dd.opts.ConfigFolderMB(), beaconID)
			store = &newStore
		}

		dd.log.Infow("", "beacon_id", beaconID, "init_dkg", "instantiating a new beacon process")
		bp, err = dd.InstantiateBeaconProcess(beaconID, *store)
		if err != nil {
			return nil, fmt.Errorf("something went wrong try to initiate DKG. err: %s", err)
		}
	}

	return bp.InitDKG(c, in)
}

// InitReshare receives information about the old and new group from which to
// operate the resharing protocol.
func (dd *DrandDaemon) InitReshare(ctx context.Context, in *drand.InitResharePacket) (*drand.GroupPacket, error) {
	bp, beaconID, err := dd.getBeaconProcess(in.GetMetadata())
	if err != nil {
		store, isStoreLoaded := dd.initialStores[beaconID]
		if !isStoreLoaded {
			dd.log.Infow("", "beacon_id", beaconID, "init_reshare", "loading store from disk")

			newStore := key.NewFileStore(dd.opts.ConfigFolderMB(), beaconID)
			store = &newStore
		}

		dd.log.Infow("", "beacon_id", beaconID, "init_reshare", "instantiating a new beacon process")
		bp, err = dd.InstantiateBeaconProcess(beaconID, *store)
		if err != nil {
			return nil, fmt.Errorf("something went wrong try to initiate DKG")
		}
	}

	return bp.InitReshare(ctx, in)
}

// PingPong simply responds with an empty packet, proving that this drand node
// is up and alive.
func (dd *DrandDaemon) PingPong(ctx context.Context, in *drand.Ping) (*drand.Pong, error) {
	metadata := common.NewMetadata(dd.version.ToProto())
	return &drand.Pong{Metadata: metadata}, nil
}

// Status responds with the actual status of drand process
func (dd *DrandDaemon) Status(ctx context.Context, in *drand.StatusRequest) (*drand.StatusResponse, error) {
	bp, _, err := dd.getBeaconProcess(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.Status(ctx, in)
}

func (dd *DrandDaemon) ListSchemes(ctx context.Context, in *drand.ListSchemesRequest) (*drand.ListSchemesResponse, error) {
	metadata := common.NewMetadata(dd.version.ToProto())

	return &drand.ListSchemesResponse{Ids: scheme.ListSchemes(), Metadata: metadata}, nil
}

// Share is a functionality of Control Service defined in protobuf/control that requests the private share of the drand node running locally
func (dd *DrandDaemon) Share(ctx context.Context, in *drand.ShareRequest) (*drand.ShareResponse, error) {
	bp, _, err := dd.getBeaconProcess(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.Share(ctx, in)
}

// PublicKey is a functionality of Control Service defined in protobuf/control
// that requests the long term public key of the drand node running locally
func (dd *DrandDaemon) PublicKey(ctx context.Context, in *drand.PublicKeyRequest) (*drand.PublicKeyResponse, error) {
	bp, _, err := dd.getBeaconProcess(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.PublicKey(ctx, in)
}

// PrivateKey is a functionality of Control Service defined in protobuf/control
// that requests the long term private key of the drand node running locally
func (dd *DrandDaemon) PrivateKey(ctx context.Context, in *drand.PrivateKeyRequest) (*drand.PrivateKeyResponse, error) {
	bp, _, err := dd.getBeaconProcess(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.PrivateKey(ctx, in)
}

// GroupFile replies with the distributed key in the response
func (dd *DrandDaemon) GroupFile(ctx context.Context, in *drand.GroupRequest) (*drand.GroupPacket, error) {
	bp, _, err := dd.getBeaconProcess(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.GroupFile(ctx, in)
}

// Shutdown stops the node
func (dd *DrandDaemon) Shutdown(ctx context.Context, in *drand.ShutdownRequest) (*drand.ShutdownResponse, error) {
	dd.Stop(ctx)

	metadata := common.NewMetadata(dd.version.ToProto())
	return &drand.ShutdownResponse{Metadata: metadata}, nil
}

// BackupDatabase triggers a backup of the primary database.
func (dd *DrandDaemon) BackupDatabase(ctx context.Context, in *drand.BackupDBRequest) (*drand.BackupDBResponse, error) {
	bp, _, err := dd.getBeaconProcess(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.BackupDatabase(ctx, in)
}

func (dd *DrandDaemon) StartFollowChain(in *drand.StartFollowRequest, stream drand.Control_StartFollowChainServer) error {
	bp, _, err := dd.getBeaconProcess(in.GetMetadata())
	if err != nil {
		return err
	}

	return bp.StartFollowChain(in, stream)

}

///////////

// Stop simply stops all drand operations.
func (dd *DrandDaemon) Stop(ctx context.Context) {
	for _, bp := range dd.beaconProcesses {
		bp.StopBeacon()
	}

	dd.state.Lock()
	if dd.pubGateway != nil {
		dd.pubGateway.StopAll(ctx)
	}
	dd.privGateway.StopAll(ctx)
	dd.control.Stop()
	dd.state.Unlock()

	dd.exitCh <- true
}

// WaitExit returns a channel that signals when drand stops its operations
func (dd *DrandDaemon) WaitExit() chan bool {
	return dd.exitCh
}
