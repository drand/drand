package core

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/drand/drand/dkg"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/common"
	dhttp "github.com/drand/drand/http"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/metrics/pprof"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
)

type DrandDaemon struct {
	initialStores   map[string]*key.Store
	beaconProcesses map[string]*BeaconProcess
	// hex encoded chainHash mapping to beaconID
	chainHashes map[string]string

	privGateway *net.PrivateGateway
	pubGateway  *net.PublicGateway
	control     net.ControlListener

	dkg *dkg.DKGProcess

	handler *dhttp.DrandHandler

	opts *Config
	log  log.Logger

	// global state lock
	state         sync.Mutex
	completedDKGs chan dkg.SharingOutput
	exitCh        chan bool

	// version indicates the base code variant
	version common.Version
}

// NewDrandDaemon creates a new instance of DrandDaemon
func NewDrandDaemon(c *Config) (*DrandDaemon, error) {
	logger := c.Logger()
	if !c.insecure && (c.certPath == "" || c.keyPath == "") {
		return nil, errors.New("config: need to set WithInsecure if no certificate and private key path given")
	}

	drandDaemon := &DrandDaemon{
		opts:            c,
		log:             logger,
		exitCh:          make(chan bool, 1),
		completedDKGs:   make(chan dkg.SharingOutput),
		version:         common.GetAppVersion(),
		initialStores:   make(map[string]*key.Store),
		beaconProcesses: make(map[string]*BeaconProcess),
		chainHashes:     make(map[string]string),
	}

	// Add callback to register a new handler for http server after finishing DKG successfully
	c.dkgCallback = func(share *key.Share, group *key.Group) {
		beaconID := common.GetCanonicalBeaconID(group.ID)

		drandDaemon.state.Lock()
		bp, isPresent := drandDaemon.beaconProcesses[beaconID]
		drandDaemon.state.Unlock()

		if isPresent {
			drandDaemon.AddBeaconHandler(beaconID, bp)
		}
	}

	if err := drandDaemon.init(); err != nil {
		return nil, err
	}

	metrics.DrandStorageBackend.
		WithLabelValues(string(c.dbStorageEngine)).
		Set(float64(chain.MetricsStorageType(c.dbStorageEngine)))

	return drandDaemon, nil
}

func (dd *DrandDaemon) RemoteStatus(ctx context.Context, request *drand.RemoteStatusRequest) (*drand.RemoteStatusResponse, error) {
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

func (dd *DrandDaemon) init() error {
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
	ctx := log.ToContext(context.Background(), lg)

	var err error
	dd.log.Infow("", "network", "init", "insecure", c.insecure)

	handler, err := dhttp.New(ctx, c.Version())
	if err != nil {
		return err
	}

	if pubAddr != "" {
		if dd.pubGateway, err = net.NewRESTPublicGateway(ctx, pubAddr, c.certPath, c.keyPath, c.certmanager,
			handler.GetHTTPHandler(), c.insecure); err != nil {
			return err
		}
	}

	// set up the gRPC clients
	p := c.ControlPort()
	controlListener, err := net.NewGRPCListenerWithLogger(lg, dd, p)
	if err != nil {
		return err
	}
	dd.control = controlListener

	dd.handler = handler
	dd.privGateway, err = net.NewGRPCPrivateGateway(ctx, privAddr, c.certPath, c.keyPath, c.certmanager, dd, c.insecure, c.grpcOpts...)
	if err != nil {
		return err
	}
	dkgStore, err := dkg.NewDKGStore(c.configFolder, c.boltOpts)
	if err != nil {
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
func (dd *DrandDaemon) InstantiateBeaconProcess(beaconID string, store key.Store) (*BeaconProcess, error) {
	beaconID = common.GetCanonicalBeaconID(beaconID)
	// we add the BeaconID to our logger's name. Notice the BeaconID never changes.
	logger := dd.log.Named(beaconID)
	bp, err := NewBeaconProcess(logger, store, dd.completedDKGs, beaconID, dd.opts, dd.privGateway, dd.pubGateway)
	if err != nil {
		return nil, err
	}
	go bp.StartListeningForDKGUpdates()

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
func (dd *DrandDaemon) RemoveBeaconProcess(beaconID string, bp *BeaconProcess) {
	beaconID = common.GetCanonicalBeaconID(beaconID)

	chainHash := ""
	if bp.group != nil {
		info := chain.NewChainInfoWithLogger(dd.log, bp.group)
		chainHash = info.HashString()
	}

	dd.state.Lock()

	delete(dd.beaconProcesses, beaconID)
	delete(dd.chainHashes, chainHash)
	if common.IsDefaultBeaconID(beaconID) {
		delete(dd.chainHashes, common.DefaultChainHash)
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
func (dd *DrandDaemon) AddBeaconHandler(beaconID string, bp *BeaconProcess) {
	chainHash := chain.NewChainInfoWithLogger(dd.log, bp.group).HashString()

	bh := dd.handler.RegisterNewBeaconHandler(&drandProxy{bp}, chainHash)

	dd.state.Lock()
	dd.chainHashes[chainHash] = beaconID
	dd.state.Unlock()

	if common.IsDefaultBeaconID(beaconID) {
		dd.handler.RegisterDefaultBeaconHandler(bh)

		dd.state.Lock()
		dd.chainHashes[common.DefaultChainHash] = beaconID
		dd.state.Unlock()
	}
}

// RemoveBeaconHandler removes a handler linked to beacon with chain hash from http server used to
// expose public services
func (dd *DrandDaemon) RemoveBeaconHandler(beaconID string, bp *BeaconProcess) {
	if bp.group == nil {
		return
	}

	info := chain.NewChainInfoWithLogger(dd.log, bp.group)
	dd.handler.RemoveBeaconHandler(info.HashString())
	if common.IsDefaultBeaconID(beaconID) {
		dd.handler.RemoveBeaconHandler(common.DefaultChainHash)
	}
}

// LoadBeaconsFromDisk checks for existing stores and creates the corresponding BeaconProcess
// accordingly to each stored BeaconID.
// When singleBeacon is set, and the singleBeaconName matches one of the stored beacons, then
// only that beacon will be loaded.
// If the singleBeaconName is an empty string, no beacon will be loaded.
func (dd *DrandDaemon) LoadBeaconsFromDisk(metricsFlag string, singleBeacon bool, singleBeaconName string) error {
	// Are we trying to start the daemon without any beacon running?
	if singleBeacon && singleBeaconName == "" {
		dd.log.Warnw("starting daemon with no active beacon")
		return nil
	}

	// Load possible existing stores
	stores, err := key.NewFileStores(dd.opts.ConfigFolderMB())
	if err != nil {
		return err
	}

	metricsHandlers := make([]metrics.Handler, 0, len(stores))

	startedAtLeastOne := false
	for beaconID, fs := range stores {
		if singleBeacon && singleBeaconName != beaconID {
			continue
		}

		bp, err := dd.LoadBeaconFromStore(beaconID, fs)
		if err != nil {
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
		_ = metrics.StartWithLogger(dd.log, metricsFlag, pprof.WithProfile(), metricsHandlers)
	}

	return nil
}

func (dd *DrandDaemon) LoadBeaconFromDisk(beaconID string) (*BeaconProcess, error) {
	store := key.NewFileStore(dd.opts.ConfigFolderMB(), beaconID)
	return dd.LoadBeaconFromStore(beaconID, store)
}

func (dd *DrandDaemon) LoadBeaconFromStore(beaconID string, store key.Store) (*BeaconProcess, error) {
	bp, err := dd.InstantiateBeaconProcess(beaconID, store)
	if err != nil {
		dd.log.Errorw("can't instantiate randomness beacon", "beacon id", beaconID, "err", err)
		return nil, err
	}

	status, err := dd.dkg.DKGStatus(context.Background(), &drand.DKGStatusRequest{BeaconID: beaconID})
	if err != nil {
		return nil, err
	}

	freshRun := status.Complete == nil
	if freshRun {
		dd.log.Infow(fmt.Sprintf("beacon id [%s]: will run as fresh install -> expect to run DKG.", beaconID))
		return bp, nil
	}

	if err := bp.Load(); err != nil {
		return nil, err
	}
	dd.log.Infow(fmt.Sprintf("beacon id [%s]: will start running randomness beacon.", beaconID))

	// Add beacon handler for http server
	dd.AddBeaconHandler(beaconID, bp)

	err = bp.StartBeacon(true)

	return bp, err
}
