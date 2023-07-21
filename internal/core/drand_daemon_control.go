package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/drand/drand/common/key"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/internal/metrics"
	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
)

// PingPong simply responds with an empty packet, proving that this drand node
// is up and alive.
func (dd *DrandDaemon) PingPong(ctx context.Context, _ *drand.Ping) (*drand.Pong, error) {
	_, span := metrics.NewSpan(ctx, "dd.PingPong")
	defer span.End()
	metadata := common.NewMetadata(dd.version.ToProto())
	return &drand.Pong{Metadata: metadata}, nil
}

// Status responds with the actual status of drand process
func (dd *DrandDaemon) Status(ctx context.Context, in *drand.StatusRequest) (*drand.StatusResponse, error) {
	ctx, span := metrics.NewSpan(ctx, "dd.Status")
	defer span.End()

	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.Status(ctx, in)
}

func (dd *DrandDaemon) ListSchemes(ctx context.Context, _ *drand.ListSchemesRequest) (*drand.ListSchemesResponse, error) {
	_, span := metrics.NewSpan(ctx, "dd.ListSchemes")
	defer span.End()

	metadata := common.NewMetadata(dd.version.ToProto())

	return &drand.ListSchemesResponse{Ids: crypto.ListSchemes(), Metadata: metadata}, nil
}

// PublicKey is a functionality of Control Service defined in protobuf/control
// that requests the long term public key of the drand node running locally
func (dd *DrandDaemon) PublicKey(ctx context.Context, in *drand.PublicKeyRequest) (*drand.PublicKeyResponse, error) {
	ctx, span := metrics.NewSpan(ctx, "dd.PublicKey")
	defer span.End()

	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.PublicKey(ctx, in)
}

// GroupFile replies with the distributed key in the response
func (dd *DrandDaemon) GroupFile(ctx context.Context, in *drand.GroupRequest) (*drand.GroupPacket, error) {
	ctx, span := metrics.NewSpan(ctx, "dd.GroupFile")
	defer span.End()

	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.GroupFile(ctx, in)
}

// Shutdown stops the node
func (dd *DrandDaemon) Shutdown(ctx context.Context, in *drand.ShutdownRequest) (*drand.ShutdownResponse, error) {
	ctx, span := metrics.NewSpan(ctx, "dd.Shutdown")
	defer span.End()

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

		dd.RemoveBeaconHandler(ctx, beaconID, bp)

		bp.Stop(ctx)
		<-bp.WaitExit()

		dd.RemoveBeaconProcess(ctx, beaconID, bp)
	}

	metadata := common.NewMetadata(dd.version.ToProto())
	metadata.BeaconID = in.GetMetadata().GetBeaconID()
	return &drand.ShutdownResponse{Metadata: metadata}, nil
}

// LoadBeacon tells the DrandDaemon to load a new beacon into the memory
func (dd *DrandDaemon) LoadBeacon(ctx context.Context, in *drand.LoadBeaconRequest) (*drand.LoadBeaconResponse, error) {
	ctx, span := metrics.NewSpan(ctx, "dd.LoadBeacon")
	defer span.End()

	beaconID, err := dd.readBeaconID(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	_, err = dd.getBeaconProcessByID(beaconID)
	if err == nil {
		return nil, fmt.Errorf("beacon id [%s] is already running", beaconID)
	}

	_, err = dd.LoadBeaconFromDisk(ctx, beaconID)
	if err != nil {
		return nil, err
	}

	metadata := common.NewMetadata(dd.version.ToProto())
	return &drand.LoadBeaconResponse{Metadata: metadata}, nil
}

// BackupDatabase triggers a backup of the primary database.
func (dd *DrandDaemon) BackupDatabase(ctx context.Context, in *drand.BackupDBRequest) (*drand.BackupDBResponse, error) {
	ctx, span := metrics.NewSpan(ctx, "dd.BackupDatabase")
	defer span.End()

	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.BackupDatabase(ctx, in)
}

func (dd *DrandDaemon) StartFollowChain(in *drand.StartSyncRequest, stream drand.Control_StartFollowChainServer) error {
	ctx, span := metrics.NewSpan(stream.Context(), "dd.StartFollowChain")
	defer span.End()

	dd.log.Debugw("StartFollowChain", "requested_chainhash", in.Metadata.ChainHash)
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return err
	}

	return bp.StartFollowChain(ctx, in, stream)
}

func (dd *DrandDaemon) StartCheckChain(in *drand.StartSyncRequest, stream drand.Control_StartCheckChainServer) error {
	dd.log.Debugw("StartCheckChain", "requested_chainhash", in.Metadata.ChainHash)
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return err
	}

	return bp.StartCheckChain(in, stream)
}

func (dd *DrandDaemon) ListBeaconIDs(ctx context.Context, _ *drand.ListBeaconIDsRequest) (*drand.ListBeaconIDsResponse, error) {
	_, span := metrics.NewSpan(ctx, "dd.ListBeaconIDs")
	defer span.End()

	metadata := common.NewMetadata(dd.version.ToProto())

	dd.state.Lock()
	defer dd.state.Unlock()

	ids := make([]string, 0)
	for id := range dd.beaconProcesses {
		ids = append(ids, id)
	}

	return &drand.ListBeaconIDsResponse{Ids: ids, Metadata: metadata}, nil
}

func (dd *DrandDaemon) KeypairFor(beaconID string) (*key.Pair, error) {
	bp, exists := dd.beaconProcesses[beaconID]
	if !exists {
		return nil, fmt.Errorf("no beacon found for ID %s", beaconID)
	}

	return bp.priv, nil
}

// /////////

// Stop simply stops all drand operations.
func (dd *DrandDaemon) Stop(ctx context.Context) {
	ctx, span := metrics.NewSpan(ctx, "dd.Stop")
	defer span.End()

	dd.log.Debugw("dd.Stop called")
	select {
	case <-dd.exitCh:
		msg := "trying to stop an already stopping daemon"
		dd.log.Errorw(msg)
		span.RecordError(errors.New(msg))
		return
	default:
		dd.log.Infow("Stopping DrandDaemon")
	}

	dd.dkg.Close()

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
			err := fmt.Errorf("beacon process %q failed to terminate in 5 seconds, exiting forcefully", bp.getBeaconID())
			span.RecordError(err)
		}
	}

	dd.log.Debugw("all beacons exited successfully")

	if dd.pubGateway != nil {
		dd.pubGateway.StopAll(ctx)
		dd.log.Debugw("pubGateway stopped successfully")
	}
	dd.privGateway.StopAll(ctx)
	dd.log.Debugw("privGateway stopped successfully")

	// We launch this in a goroutine to allow the stop connection to exit successfully.
	// If we wouldn't launch it in a goroutine the Stop call itself would block the shutdown
	// procedure and we'd be in a loop.
	// By default, the Stop call will try to terminate all connections nicely.
	// However, after a timeout, it will forcefully close all connections and terminate.
	go func() {
		dd.state.Lock()
		defer dd.state.Unlock()
		dd.control.Stop()
		dd.log.Debugw("control stopped successfully")
	}()

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
