package core

import (
	"context"
	"fmt"
	"time"

	"github.com/drand/drand/crypto"
	"github.com/drand/drand/key"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
)

// InitDKG take a InitDKGPacket, extracts the informations needed and wait for
// the DKG protocol to finish. If the request specifies this node is a leader,
// it starts the DKG protocol.
func (dd *DrandDaemon) InitDKG(c context.Context, in *drand.InitDKGPacket) (*drand.GroupPacket, error) {
	beaconID, err := dd.readBeaconID(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	bp, err := dd.getBeaconProcessByID(beaconID)
	if err != nil {
		store, isStoreLoaded := dd.initialStores[beaconID]
		if !isStoreLoaded {
			dd.log.Infow("", "init_dkg", "loading store from disk")

			newStore := key.NewFileStore(dd.opts.ConfigFolderMB(), beaconID)
			store = &newStore
		}

		dd.log.Infow("", "init_dkg", "instantiating a new beacon process")
		bp, err = dd.InstantiateBeaconProcess(beaconID, *store)
		if err != nil {
			return nil, fmt.Errorf("something went wrong try to initiate DKG. err: %w", err)
		}
	}

	return bp.InitDKG(c, in)
}

// InitReshare receives information about the old and new group from which to
// operate the resharing protocol.
func (dd *DrandDaemon) InitReshare(ctx context.Context, in *drand.InitResharePacket) (*drand.GroupPacket, error) {
	beaconID, err := dd.readBeaconID(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	bp, err := dd.getBeaconProcessByID(beaconID)
	if bp == nil {
		return nil, fmt.Errorf("beacon with ID %s could not be found - make sure you have passed the id flag or have a default beacon", beaconID)
	}

	if err != nil {
		store, isStoreLoaded := dd.initialStores[beaconID]
		if !isStoreLoaded {
			dd.log.Infow("", "init_reshare", "loading store from disk")

			newStore := key.NewFileStore(dd.opts.ConfigFolderMB(), beaconID)
			store = &newStore
		}

		metrics.GroupSize.WithLabelValues(bp.getBeaconID()).Set(float64(in.Info.Nodes))
		metrics.GroupThreshold.WithLabelValues(bp.getBeaconID()).Set(float64(in.Info.Threshold))

		dd.log.Infow("", "init_reshare", "instantiating a new beacon process")
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
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.Status(ctx, in)
}

func (dd *DrandDaemon) ListSchemes(ctx context.Context, in *drand.ListSchemesRequest) (*drand.ListSchemesResponse, error) {
	metadata := common.NewMetadata(dd.version.ToProto())

	return &drand.ListSchemesResponse{Ids: crypto.ListSchemes(), Metadata: metadata}, nil
}

// Share is a functionality of Control Service defined in protobuf/control that requests the private share of
// the drand node running locally
// Deprecated: no need to export the secret share to a remote client.
func (dd *DrandDaemon) Share(_ context.Context, _ *drand.ShareRequest) (*drand.ShareResponse, error) {
	return nil, fmt.Errorf("deprecated function: exporting the Share to a remote client is not supported")
}

// PublicKey is a functionality of Control Service defined in protobuf/control
// that requests the long term public key of the drand node running locally
func (dd *DrandDaemon) PublicKey(ctx context.Context, in *drand.PublicKeyRequest) (*drand.PublicKeyResponse, error) {
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.PublicKey(ctx, in)
}

// PrivateKey is a functionality of Control Service defined in protobuf/control
// that requests the long term private key of the drand node running locally
// Deprecated: no need to export secret key to a remote client.
func (dd *DrandDaemon) PrivateKey(ctx context.Context, in *drand.PrivateKeyRequest) (*drand.PrivateKeyResponse, error) {
	return nil, fmt.Errorf("deprecated function: exporting the PrivateKey to a remote client is not supported")
}

// GroupFile replies with the distributed key in the response
func (dd *DrandDaemon) GroupFile(ctx context.Context, in *drand.GroupRequest) (*drand.GroupPacket, error) {
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.GroupFile(ctx, in)
}

// Shutdown stops the node
func (dd *DrandDaemon) Shutdown(ctx context.Context, in *drand.ShutdownRequest) (*drand.ShutdownResponse, error) {
	// If beacon id is empty, we will stop the entire node. Otherwise, we will stop the specific beacon process
	if in.GetMetadata().GetBeaconID() == "" {
		dd.Stop(ctx)
	} else {
		beaconID, err := dd.readBeaconID(in.GetMetadata())
		if err != nil {
			return nil, err
		}

		bp, err := dd.getBeaconProcessByID(beaconID)
		if err != nil {
			return nil, err
		}

		dd.RemoveBeaconHandler(beaconID, bp)

		bp.Stop(ctx)
		<-bp.WaitExit()

		dd.RemoveBeaconProcess(beaconID, bp)
	}

	metadata := common.NewMetadata(dd.version.ToProto())
	metadata.BeaconID = in.GetMetadata().GetBeaconID()
	return &drand.ShutdownResponse{Metadata: metadata}, nil
}

// LoadBeacon tells the DrandDaemon to load a new beacon into the memory
func (dd *DrandDaemon) LoadBeacon(ctx context.Context, in *drand.LoadBeaconRequest) (*drand.LoadBeaconResponse, error) {
	beaconID, err := dd.readBeaconID(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	_, err = dd.getBeaconProcessByID(beaconID)
	if err == nil {
		return nil, fmt.Errorf("beacon id [%s] is already running", beaconID)
	}

	_, err = dd.LoadBeaconFromDisk(beaconID)
	if err != nil {
		return nil, err
	}

	metadata := common.NewMetadata(dd.version.ToProto())
	return &drand.LoadBeaconResponse{Metadata: metadata}, nil
}

// BackupDatabase triggers a backup of the primary database.
func (dd *DrandDaemon) BackupDatabase(ctx context.Context, in *drand.BackupDBRequest) (*drand.BackupDBResponse, error) {
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.BackupDatabase(ctx, in)
}

func (dd *DrandDaemon) StartFollowChain(in *drand.StartSyncRequest, stream drand.Control_StartFollowChainServer) error {
	dd.log.Debugw("StartFollowChain", "requested_chainhash", in.Metadata.ChainHash)
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return err
	}

	return bp.StartFollowChain(in, stream)
}

func (dd *DrandDaemon) StartCheckChain(in *drand.StartSyncRequest, stream drand.Control_StartCheckChainServer) error {
	dd.log.Debugw("StartCheckChain", "requested_chainhash", in.Metadata.ChainHash)
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return err
	}

	return bp.StartCheckChain(in, stream)
}

func (dd *DrandDaemon) ListBeaconIDs(ctx context.Context, in *drand.ListBeaconIDsRequest) (*drand.ListBeaconIDsResponse, error) {
	metadata := common.NewMetadata(dd.version.ToProto())

	dd.state.Lock()
	defer dd.state.Unlock()

	ids := make([]string, 0)
	for id := range dd.beaconProcesses {
		ids = append(ids, id)
	}

	return &drand.ListBeaconIDsResponse{Ids: ids, Metadata: metadata}, nil
}

// /////////

// Stop simply stops all drand operations.
func (dd *DrandDaemon) Stop(ctx context.Context) {
	dd.log.Debugw("dd.Stop called")
	select {
	case <-dd.exitCh:
		dd.log.Errorw("Trying to stop an already stopping daemon")
		return
	default:
		dd.log.Infow("Stopping DrandDaemon")
	}
	for _, bp := range dd.beaconProcesses {
		dd.log.Debugw("Sending Stop to beaconProcesses", "id", bp.getBeaconID())
		bp.Stop(ctx)
	}

	for _, bp := range dd.beaconProcesses {
		dd.log.Debugw("waiting for beaconProcess to finish", "id", bp.getBeaconID())

		//nolint:gomnd // We want to wait for 5 seconds before sending a timeout for the beacon shutdown
		t := time.NewTimer(5 * time.Second)
		select {
		case <-bp.WaitExit():
			if !t.Stop() {
				<-t.C
			}
		case <-t.C:
			dd.log.Errorw("beacon process failed to terminate in 5 seconds, exiting forcefully", "id", bp.getBeaconID())
		}
	}

	dd.log.Debugw("all beacons exited successfully")

	if dd.pubGateway != nil {
		dd.pubGateway.StopAll(ctx)
		dd.log.Debugw("pubGateway stopped successfully")
	}
	dd.privGateway.StopAll(ctx)
	dd.log.Debugw("privGateway stopped successfully")
	// we defer the stop of the ControlListener to avoid canceling our context already
	defer func() {
		// We launch this in a goroutine to allow the stop connection to exit successfully.
		// If we wouldn't launch it in a goroutine the Stop call itself would block the shutdown
		// procedure and we'd be in a loop.
		// By default, the Stop call will try to terminate all connections nicely.
		// However, after a timeout, it will forcefully close all connections and terminate.
		go func() {
			dd.control.Stop()
			dd.log.Debugw("control stopped successfully")
		}()
	}()

	dd.log.Debugw("waiting for dd.exitCh to finish")

	select {
	case dd.exitCh <- true:
		dd.log.Debugw("signaled dd.exitCh")
		close(dd.exitCh)
	case <-ctx.Done():
		dd.log.Warnw("Context canceled, DrandDaemon exitCh probably blocked")
		close(dd.exitCh)
	}
}

// WaitExit returns a channel that signals when drand stops its operations
func (dd *DrandDaemon) WaitExit() chan bool {
	return dd.exitCh
}
