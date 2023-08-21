package core

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sync"

	"go.opentelemetry.io/otel/attribute"

	common2 "github.com/drand/drand/common"
	chain2 "github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/key"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/chain"
	"github.com/drand/drand/internal/dkg"
	dhttp "github.com/drand/drand/internal/http"
	"github.com/drand/drand/internal/metrics"
	"github.com/drand/drand/internal/metrics/pprof"
	"github.com/drand/drand/internal/net"
	"github.com/drand/drand/protobuf/drand"
)

type DrandDaemon struct {
	beaconProcesses map[string]*BeaconProcess
	// hex encoded chainHash mapping to beaconID
	chainHashes map[string]string

	privGateway *net.PrivateGateway
	pubGateway  *net.PublicGateway
	control     net.ControlListener

	dkg *dkg.Process

	handler *dhttp.DrandHandler

	opts *Config
	log  log.Logger

	// global state lock
	state         sync.Mutex
	completedDKGs chan dkg.SharingOutput
	exitCh        chan bool

	// version indicates the base code variant
	version common2.Version
}

// NewDrandDaemon creates a new instance of DrandDaemon
func NewDrandDaemon(ctx context.Context, c *Config) (*DrandDaemon, error) {
	ctx, span := metrics.NewSpan(ctx, "NewDrandDaemon")
	defer span.End()

	logger := c.Logger()
	if !c.insecure && (c.certPath == "" || c.keyPath == "") {
		err := errors.New("config: need to set WithInsecure if no certificate and private key path given")
		span.RecordError(err)
		return nil, err
	}

	drandDaemon := &DrandDaemon{
		opts:            c,
		log:             logger,
		exitCh:          make(chan bool, 1),
		completedDKGs:   make(chan dkg.SharingOutput),
		version:         common2.GetAppVersion(),
		beaconProcesses: make(map[string]*BeaconProcess),
		chainHashes:     make(map[string]string),
	}

	// Add callback to register a new handler for http server after finishing DKG successfully
	c.dkgCallback = func(ctx context.Context, share *key.Share, group *key.Group) {
		beaconID := common2.GetCanonicalBeaconID(group.ID)

		drandDaemon.state.Lock()
		bp, isPresent := drandDaemon.beaconProcesses[beaconID]
		drandDaemon.state.Unlock()

		if isPresent {
			drandDaemon.AddBeaconHandler(ctx, beaconID, bp)
		}
	}

	if err := drandDaemon.init(ctx); err != nil {
		return nil, err
	}

	metrics.DrandStorageBackend.
		WithLabelValues(string(c.dbStorageEngine)).
		Set(float64(chain.MetricsStorageType(c.dbStorageEngine)))

	return drandDaemon, nil
}

func (dd *DrandDaemon) RemoteStatus(ctx context.Context, request *drand.RemoteStatusRequest) (*drand.RemoteStatusResponse, error) {
	ctx, span := metrics.NewSpan(ctx, "dd.RemoteStatus")
	defer span.End()

	beaconID, err := dd.readBeaconID(request.Metadata)
	if err != nil {
		return nil, err
	}

	bp, err := dd.getBeaconProcessByID(beaconID)
	if err != nil {
		return nil, err
	}

	return bp.RemoteStatus(ctx, request)
}

func (dd *DrandDaemon) init(ctx context.Context) error {
	ctx, span := metrics.NewSpan(ctx, "dd.init")
	defer span.End()

	dd.state.Lock()
	defer dd.state.Unlock()
	c := dd.opts

	// Set the private API address to the command-line flag, if given.
	// Otherwise, set it to the address associated with stored private key.
	privAddr := c.PrivateListenAddress("")
	pubAddr := c.PublicListenAddress("")

	if privAddr == "" {
		return fmt.Errorf("private listen address cannot be empty")
	}

	// we set our logger name to its node address
	dd.log = dd.log.Named(privAddr)

	// ctx is used to create the gateway below.
	// Gateway constructors (specifically, the generated gateway stubs that require it)
	// do not actually use it, so we are passing a background context to be safe.
	lg := dd.log.With("server", "http")
	ctx = log.ToContext(ctx, lg)

	var err error
	dd.log.Infow("", "network", "init", "insecure", c.insecure)

	handler, err := dhttp.New(ctx, c.Version())
	if err != nil {
		span.RecordError(err)
		return err
	}

	if pubAddr != "" {
		if dd.pubGateway, err = net.NewRESTPublicGateway(ctx, pubAddr, c.certPath, c.keyPath, c.certmanager,
			handler.GetHTTPHandler(), c.insecure); err != nil {
			span.RecordError(err)
			return err
		}
	}

	// set up the gRPC clients
	p := c.ControlPort()
	controlListener, err := net.NewGRPCListener(lg, dd, p)
	if err != nil {
		return err
	}
	dd.control = controlListener

	dd.handler = handler
	dd.privGateway, err = net.NewGRPCPrivateGateway(ctx, privAddr, c.certPath, c.keyPath, c.certmanager, dd, c.insecure, c.grpcOpts...)
	if err != nil {
		span.RecordError(err)
		return err
	}
	dkgStore, err := dkg.NewDKGStore(c.configFolder, c.boltOpts)
	if err != nil {
		span.RecordError(err)
		return err
	}

	dkgConfig := dkg.Config{
		TimeBetweenDKGPhases: c.dkgPhaseTimeout,
		KickoffGracePeriod:   c.dkgKickoffGracePeriod,
		SkipKeyVerification:  false,
	}
	dd.dkg = dkg.NewDKGProcess(dkgStore, dd, dd.completedDKGs, dd.privGateway, dkgConfig, dd.log)

	go dd.control.Start()

	dd.log.Infow("DrandDaemon initialized",
		"private_listen", privAddr,
		"control_port", c.ControlPort(),
		"public_listen", pubAddr,
		"folder", c.ConfigFolderMB())
	dd.privGateway.StartAll()
	if dd.pubGateway != nil {
		dd.pubGateway.StartAll()
	}

	return nil
}

// InstantiateBeaconProcess creates a new BeaconProcess linked to beacon with id 'beaconID'
func (dd *DrandDaemon) InstantiateBeaconProcess(ctx context.Context, beaconID string, store key.Store) (*BeaconProcess, error) {
	ctx, span := metrics.NewSpan(ctx, "dd.InstantiateBeaconProcess")
	defer span.End()

	beaconID = common2.GetCanonicalBeaconID(beaconID)
	// we add the BeaconID to our logger's name. Notice the BeaconID never changes.
	logger := dd.log.Named(beaconID)
	bp, err := NewBeaconProcess(ctx, logger, store, dd.completedDKGs, beaconID, dd.opts, dd.privGateway, dd.pubGateway)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	go bp.StartListeningForDKGUpdates(ctx)

	dd.state.Lock()
	dd.beaconProcesses[beaconID] = bp
	dd.state.Unlock()

	metrics.DKGStateChange(metrics.DKGNotStarted, beaconID, false)
	metrics.ReshareStateChange(metrics.ReshareIdle, beaconID, false)
	metrics.IsDrandNode.Set(1)
	metrics.DrandStartTimestamp.SetToCurrentTime()

	return bp, nil
}

// RemoveBeaconProcess remove a BeaconProcess linked to beacon with id 'beaconID'
func (dd *DrandDaemon) RemoveBeaconProcess(ctx context.Context, beaconID string, bp *BeaconProcess) {
	_, span := metrics.NewSpan(ctx, "dd.RemoveBeaconProcess")
	defer span.End()

	beaconID = common2.GetCanonicalBeaconID(beaconID)

	chainHash := ""
	if bp.group != nil {
		info := chain2.NewChainInfo(dd.log, bp.group)
		chainHash = info.HashString()
	}

	dd.state.Lock()

	delete(dd.beaconProcesses, beaconID)
	delete(dd.chainHashes, chainHash)
	if common2.IsDefaultBeaconID(beaconID) {
		delete(dd.chainHashes, common2.DefaultChainHash)
	}

	dd.log.Debugw("BeaconProcess removed", "beacon_id", beaconID, "chain_hash", chainHash)

	metrics.DKGStateChange(metrics.DKGShutdown, beaconID, false)
	metrics.ReshareStateChange(metrics.ReshareShutdown, beaconID, false)
	metrics.IsDrandNode.Set(1)
	metrics.DrandStartTimestamp.SetToCurrentTime()

	dd.state.Unlock()
}

// AddBeaconHandler adds a handler linked to beacon with chain hash from http server used to
// expose public services
func (dd *DrandDaemon) AddBeaconHandler(ctx context.Context, beaconID string, bp *BeaconProcess) {
	_, span := metrics.NewSpan(ctx, "dd.AddBeaconHandler")
	defer span.End()

	chainHash := chain2.NewChainInfo(dd.log, bp.group).HashString()

	bh := dd.handler.RegisterNewBeaconHandler(&drandProxy{bp}, chainHash)

	dd.state.Lock()
	dd.chainHashes[chainHash] = beaconID
	dd.state.Unlock()

	if common2.IsDefaultBeaconID(beaconID) {
		dd.handler.RegisterDefaultBeaconHandler(bh)

		dd.state.Lock()
		dd.chainHashes[common2.DefaultChainHash] = beaconID
		dd.state.Unlock()
	}
}

// RemoveBeaconHandler removes a handler linked to beacon with chain hash from http server used to
// expose public services
func (dd *DrandDaemon) RemoveBeaconHandler(ctx context.Context, beaconID string, bp *BeaconProcess) {
	_, span := metrics.NewSpan(ctx, "dd.RemoveBeaconHandler")
	defer span.End()

	if bp.group == nil {
		return
	}

	info := chain2.NewChainInfo(dd.log, bp.group)
	dd.handler.RemoveBeaconHandler(info.HashString())
	if common2.IsDefaultBeaconID(beaconID) {
		dd.handler.RemoveBeaconHandler(common2.DefaultChainHash)
	}
}

// LoadBeaconsFromDisk checks for existing stores and creates the corresponding BeaconProcess
// accordingly to each stored BeaconID.
// When singleBeacon is set, and the singleBeaconName matches one of the stored beacons, then
// only that beacon will be loaded.
// If the singleBeaconName is an empty string, no beacon will be loaded.
func (dd *DrandDaemon) LoadBeaconsFromDisk(ctx context.Context, metricsFlag string, singleBeacon bool, singleBeaconName string) error {
	ctx, span := metrics.NewSpan(ctx, "dd.LoadBeaconsFromDisk")
	defer span.End()

	// Are we trying to start the daemon without any beacon running?
	if singleBeacon && singleBeaconName == "" {
		dd.log.Warnw("starting daemon with no active beacon")
		span.SetAttributes(
			attribute.Bool("noBeacon", true),
		)
		return nil
	}

	// Load possible existing stores
	stores, err := key.NewFileStores(dd.opts.ConfigFolderMB())
	if err != nil {
		span.RecordError(err)
		return err
	}

	metricsHandlers := make([]metrics.Handler, 0, len(stores))

	startedAtLeastOne := false
	for beaconID, fs := range stores {
		if singleBeacon && singleBeaconName != beaconID {
			continue
		}

		bp, err := dd.LoadBeaconFromStore(ctx, beaconID, fs)
		if err != nil {
			span.RecordError(err)
			return err
		}

		if metricsFlag != "" {
			bp.log.Infow("", "metrics", "adding handler")
			metricsHandlers = append(metricsHandlers, bp.MetricsHandlerForPeer)
		}

		startedAtLeastOne = true
	}

	if !startedAtLeastOne {
		dd.log.Warnw("starting daemon with no active beacon")
	}

	// Start metrics server
	if len(metricsHandlers) > 0 {
		_ = metrics.Start(dd.log, metricsFlag, pprof.WithProfile(), metricsHandlers)
	}

	return nil
}

func (dd *DrandDaemon) LoadBeaconFromDisk(ctx context.Context, beaconID string) (*BeaconProcess, error) {
	ctx, span := metrics.NewSpan(ctx, "dd.LoadBeaconFromDisk")
	defer span.End()

	store := key.NewFileStore(dd.opts.ConfigFolderMB(), beaconID)
	return dd.LoadBeaconFromStore(ctx, beaconID, store)
}

func (dd *DrandDaemon) LoadBeaconFromStore(ctx context.Context, beaconID string, store key.Store) (*BeaconProcess, error) {
	ctx, span := metrics.NewSpan(ctx, "dd.LoadBeaconFromStore")
	defer span.End()

	bp, err := dd.InstantiateBeaconProcess(ctx, beaconID, store)
	if err != nil {
		dd.log.Errorw("can't instantiate randomness beacon", "beacon id", beaconID, "err", err)
		span.RecordError(err)
		return nil, err
	}

	status, err := dd.dkg.DKGStatus(ctx, &drand.DKGStatusRequest{BeaconID: beaconID})
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	freshRun := status.Complete == nil
	if freshRun {
		// migration path from v1-> v2
		// if there is a group file but no DKG status in the DB, we perform the migration
		g, err := store.LoadGroup()

		// by default, no group returns a `no such file or directory` error, which we want to ignore
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}

		if g == nil {
			dd.log.Infow(fmt.Sprintf("beacon id [%s]: will run as fresh install -> expect to run DKG.", beaconID))
			return bp, nil
		}

		share, err := store.LoadShare(g.Scheme)
		if err != nil {
			return nil, err
		}

		if err := dd.dkg.Migrate(beaconID, g, share); err != nil {
			return nil, err
		}
	}

	if err := bp.Load(ctx); err != nil {
		return nil, err
	}
	dd.log.Infow(fmt.Sprintf("beacon id [%s]: will start running randomness beacon.", beaconID))

	// Add beacon handler for http server
	dd.AddBeaconHandler(ctx, beaconID, bp)

	err = bp.StartBeacon(ctx, true)
	if err != nil {
		span.RecordError(err)
	}
	return bp, err
}
