package node

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/util"

	"github.com/kabukky/httpscerts"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client/grpc"
	"github.com/drand/drand/core"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/demo/cfg"
	"github.com/drand/drand/fs"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"
)

// LocalNode ...
type LocalNode struct {
	base       string
	i          int
	period     string
	beaconID   string
	scheme     *crypto.Scheme
	logPath    string
	privAddr   string
	pubAddr    string
	ctrlAddr   string
	ctrlClient *net.ControlClient
	dkgRunner  *test.DKGRunner
	tls        bool
	priv       *key.Pair

	dbEngineType chain.StorageType
	pgDSN        func() string
	memDBSize    int

	log log.Logger

	daemon *core.DrandDaemon
}

func NewLocalNode(i int, bindAddr string, cfg cfg.Config) *LocalNode {
	nbase := path.Join(cfg.BasePath, fmt.Sprintf("node-%d", i))
	_ = os.MkdirAll(nbase, 0740)
	logPath := path.Join(nbase, "log")

	lg := log.New(nil, log.DebugLevel, false)

	// make certificates for the node.
	err := httpscerts.Generate(
		path.Join(nbase, fmt.Sprintf("server-%d.crt", i)),
		path.Join(nbase, fmt.Sprintf("server-%d.key", i)),
		bindAddr)
	if err != nil {
		return nil
	}

	controlAddr := test.FreeBind(bindAddr)
	dkgClient, err := net.NewDKGControlClientWithLogger(lg, controlAddr)
	if err != nil {
		return nil
	}

	l := &LocalNode{
		base:         nbase,
		i:            i,
		period:       cfg.Period,
		tls:          cfg.WithTLS,
		logPath:      logPath,
		log:          lg,
		pubAddr:      test.FreeBind(bindAddr),
		privAddr:     test.FreeBind(bindAddr),
		ctrlAddr:     controlAddr,
		scheme:       cfg.Scheme,
		beaconID:     cfg.BeaconID,
		dbEngineType: cfg.DBEngineType,
		pgDSN:        cfg.PgDSN,
		memDBSize:    cfg.MemDBSize,
		dkgRunner:    &test.DKGRunner{BeaconID: cfg.BeaconID, Client: dkgClient},
	}

	var priv *key.Pair
	if l.tls {
		priv, err = key.NewTLSKeyPair(l.privAddr, l.scheme)
	} else {
		priv, err = key.NewKeyPair(l.privAddr, l.scheme)
	}
	if err != nil {
		panic(err)
	}

	l.priv = priv
	return l
}

func (l *LocalNode) Start(certFolder string, dbEngineType chain.StorageType, pgDSN func() string, memDBSize int) error {
	ctx := context.Background()

	if dbEngineType != "" {
		l.dbEngineType = dbEngineType
	}
	if pgDSN != nil {
		l.pgDSN = pgDSN
	}
	if memDBSize != 0 {
		l.memDBSize = memDBSize
	}

	certs, err := fs.Files(certFolder)
	if err != nil {
		return err
	}

	opts := []core.ConfigOption{
		core.WithConfigFolder(l.base),
		core.WithTrustedCerts(certs...),
		core.WithPublicListenAddress(l.pubAddr),
		core.WithPrivateListenAddress(l.privAddr),
		core.WithControlPort(l.ctrlAddr),
		core.WithDBStorageEngine(l.dbEngineType),
		core.WithPgDSN(l.pgDSN()),
		core.WithMemDBSize(l.memDBSize),
	}

	if l.tls {
		opts = append(opts, core.WithTLS(
			path.Join(l.base, fmt.Sprintf("server-%d.crt", l.i)),
			path.Join(l.base, fmt.Sprintf("server-%d.key", l.i))))
	} else {
		opts = append(opts, core.WithInsecure())
	}

	conf := core.NewConfigWithLogger(l.log, opts...)
	ks := key.NewFileStore(conf.ConfigFolderMB(), l.beaconID)
	err = ks.SaveKeyPair(l.priv)
	if err != nil {
		return err
	}

	err = key.Save(path.Join(l.base, "public.toml"), l.priv.Public, false)
	if err != nil {
		return err
	}

	// Create and start drand daemon
	drandDaemon, err := core.NewDrandDaemon(ctx, conf)
	if err != nil {
		return fmt.Errorf("can't instantiate drand daemon %s", err)
	}

	// Load possible existing stores
	stores, err := key.NewFileStores(conf.ConfigFolderMB())
	if err != nil {
		return err
	}

	for beaconID, ks := range stores {
		ctx := context.Background()
		bp, err := drandDaemon.InstantiateBeaconProcess(ctx, beaconID, ks)
		if err != nil {
			fmt.Printf("beacon id [%s]: can't instantiate randomness beacon. err: %s \n", beaconID, err)
			return err
		}

		err = bp.Load(ctx)
		isFreshRun := err == core.ErrDKGNotStarted
		if err != nil && !isFreshRun {
			return err
		}
		if isFreshRun {
			fmt.Printf("beacon id [%s]: will run as fresh install -> expect to run DKG.\n", beaconID)
		} else {
			fmt.Printf("beacon id [%s]: will already start running randomness beacon.\n", beaconID)
			// Add beacon handler from chain hash for http server
			drandDaemon.AddBeaconHandler(ctx, beaconID, bp)

			// XXX make it configurable so that new share holder can still start if
			// nobody started.
			// drand.StartBeacon(!c.Bool(pushFlag.Name))
			catchup := true
			err = bp.StartBeacon(ctx, catchup)
			if err != nil {
				return err
			}
		}
	}

	l.daemon = drandDaemon

	return nil
}

func (l *LocalNode) PrivateAddr() string {
	return l.privAddr
}

func (l *LocalNode) CtrlAddr() string {
	return l.ctrlAddr
}

func (l *LocalNode) PublicAddr() string {
	return l.pubAddr
}

func (l *LocalNode) Index() int {
	return l.i
}

func (l *LocalNode) ctrl() *net.ControlClient {
	if l.ctrlClient != nil {
		return l.ctrlClient
	}
	cl, err := net.NewControlClientWithLogger(l.log, l.ctrlAddr)
	if err != nil {
		l.log.Errorw("", "drand", "can't instantiate control client", "err", err)
		return nil
	}
	l.ctrlClient = cl
	return cl
}

func (l *LocalNode) StartLeaderDKG(thr int, beaconOffset int, joiners []*drand.Participant) error {
	p, err := time.ParseDuration(l.period)
	if err != nil {
		return err
	}
	timeout := 5 * time.Minute
	return l.dkgRunner.StartNetwork(thr, int(p.Seconds()), l.scheme.Name, timeout, beaconOffset, joiners)
}

func (l *LocalNode) ExecuteLeaderDKG() error {
	return l.dkgRunner.StartExecution()
}

func (l *LocalNode) WaitDKGComplete(epoch uint32, timeout time.Duration) (*key.Group, error) {
	err := l.dkgRunner.WaitForDKG(l.log, l.beaconID, epoch, int(timeout.Seconds()))
	if err != nil {
		return nil, err
	}

	groupPacket, err := l.daemon.GroupFile(context.Background(), &drand.GroupRequest{Metadata: &common.Metadata{
		BeaconID: l.beaconID,
	}})
	if err != nil {
		return nil, err
	}

	return key.GroupFromProto(groupPacket, l.scheme)
}
func (l *LocalNode) JoinDKG() error {
	return l.dkgRunner.JoinDKG()
}

func (l *LocalNode) JoinReshare(oldGroup key.Group) error {
	return l.dkgRunner.JoinReshare(&oldGroup)
}

func (l *LocalNode) GetGroup() *key.Group {
	cl := l.ctrl()

	grp, err := cl.GroupFile(l.beaconID)
	if err != nil {
		l.log.Errorw("", "drand", "can't  get group", "err", err)
		return nil
	}
	group, err := key.GroupFromProto(grp, l.scheme)
	if err != nil {
		l.log.Errorw("", "drand", "can't deserialize group", "err", err)
		return nil
	}
	return group
}

func (l *LocalNode) StartLeaderReshare(thr int, transitionTime time.Time, beaconOffset int, joiners []*drand.Participant, remainers []*drand.Participant, leavers []*drand.Participant) error {
	err := l.dkgRunner.StartProposal(thr, transitionTime, beaconOffset, joiners, remainers, leavers)
	if err != nil {
		l.log.Errorw("", "drand", "dkg run failed", "err", err)
		return err
	}

	return nil
}
func (l *LocalNode) ExecuteLeaderReshare() error {
	return l.dkgRunner.StartExecution()
}

func (l *LocalNode) AcceptReshare() error {
	err := l.dkgRunner.Accept()
	if err != nil {
		l.log.Errorw("", "drand", "dkg run failed", "err", err)
		return err
	}
	return nil
}

func (l *LocalNode) ChainInfo(_ string) bool {
	cl := l.ctrl()
	ci, err := cl.ChainInfo(l.beaconID)
	if err != nil {
		l.log.Errorw("", "drand", "can't get chain-info", "err", err)
		return false
	}
	sdist := hex.EncodeToString(ci.PublicKey)
	fmt.Printf("\t- Node %s has chain-info %s\n", l.PrivateAddr(), sdist[10:14])
	return true
}

func (l *LocalNode) Ping() bool {
	cl := l.ctrl()
	if err := cl.Ping(); err != nil {
		l.log.Errorw("", "drand", "can't ping", "err", err)
		return false
	}
	return true
}

func (l *LocalNode) GetBeacon(_ string, round uint64) (resp *drand.PublicRandResponse, cmd string) {
	cert := ""
	if l.tls {
		cert = path.Join(l.base, fmt.Sprintf("server-%d.crt", l.i))
	}
	c, _ := grpc.NewWithLogger(l.log, l.privAddr, cert, cert == "", []byte(""))

	group := l.GetGroup()
	if group == nil {
		l.log.Errorw("", "drand", "can't get group")
		return
	}

	var err error
	cmd = "unused"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := c.Get(ctx, round)
	if err != nil || r == nil {
		l.log.Errorw("", "drand", "can't get beacon", "err", err)
	}
	if r == nil {
		return
	}
	resp = &drand.PublicRandResponse{
		Round:      r.Round(),
		Signature:  r.Signature(),
		Randomness: r.Randomness(),
	}
	return
}

func (l *LocalNode) WriteCertificate(p string) {
	if l.tls {
		err := exec.Command("cp", path.Join(l.base, fmt.Sprintf("server-%d.crt", l.i)), p).Run()
		if err != nil {
			panic(err)
		}
	}
}

func (l *LocalNode) WritePublic(p string) {
	key.Save(p, l.priv.Public, false)
}

func (l *LocalNode) Stop() {
	cl := l.ctrl()
	_, err := cl.Shutdown("")
	if err != nil {
		l.log.Errorw("", "drand", "failed to shutdown", "err", err)
		return
	}
	<-l.daemon.WaitExit()
}

func (l *LocalNode) PrintLog() {
	fmt.Printf("[-] Printing logs of node %s:\n", l.PrivateAddr())
	buff, err := os.ReadFile(l.logPath)
	if err != nil {
		fmt.Printf("[-] Can't read logs at %s !\n\n", l.logPath)
		return
	}

	fmt.Printf("%s\n", string(buff))
}

func (l *LocalNode) Identity() (*drand.Participant, error) {
	keypair, err := l.daemon.KeypairFor(l.beaconID)
	if err != nil {
		return nil, err
	}
	return util.PublicKeyAsParticipant(keypair.Public)
}
