package drand

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/briandowns/spinner"
	json "github.com/nikkolasg/hexjson"
	"github.com/urfave/cli/v2"

	"github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/key"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/core"
	"github.com/drand/drand/internal/net"
	control "github.com/drand/drand/protobuf/drand"
)

type beaconIDsStatuses struct {
	Beacons map[string]*control.StatusResponse `json:"beacons"`
}

func loadCmd(c *cli.Context, l log.Logger) error {
	client, err := controlClient(c, l)
	if err != nil {
		return err
	}

	beaconID := getBeaconID(c)
	_, err = client.LoadBeacon(beaconID)
	if err != nil {
		return fmt.Errorf("could not reload the beacon process [%s]: %w", beaconID, err)
	}

	fmt.Fprintf(c.App.Writer, "Beacon process [%s] was loaded on drand.\n", beaconID)
	return nil
}

func remoteStatusCmd(c *cli.Context, l log.Logger) error {
	client, err := controlClient(c, l)
	if err != nil {
		return err
	}

	ips := c.Args().Slice()
	beaconID := getBeaconID(c)

	addresses := make([]*control.Address, len(ips))
	for i := 0; i < len(ips); i++ {
		addresses[i] = &control.Address{
			Address: ips[i],
		}
	}

	resp, err := client.RemoteStatus(c.Context, addresses, beaconID)
	if err != nil {
		return err
	}
	// set default value for all keys so json outputs something for all keys
	defaultMap := make(map[string]*control.StatusResponse)
	switch {
	case len(addresses) > 0:
		for _, addr := range addresses {
			if resp, ok := resp[addr.GetAddress()]; !ok {
				defaultMap[addr.GetAddress()] = nil
			} else {
				defaultMap[addr.GetAddress()] = resp
			}
		}
	default:
		defaultMap = resp
	}

	if c.IsSet(jsonFlag.Name) {
		str, err := json.Marshal(defaultMap)
		if err != nil {
			return fmt.Errorf("cannot marshal the response ... %w", err)
		}
		fmt.Fprintf(c.App.Writer, "%s \n", string(str))
	} else {
		for addr, resp := range defaultMap {
			fmt.Fprintf(c.App.Writer, "Status of beacon %s on node %s\n", beaconID, addr)
			if resp == nil {
				fmt.Fprintf(c.App.Writer, "\t- NO STATUS; can't connect\n")
			} else {
				fmt.Fprintf(c.App.Writer, "%s\n", core.StatusResponseToString(resp))
			}
		}
	}
	return nil
}

func pingpongCmd(c *cli.Context, l log.Logger) error {
	client, err := controlClient(c, l)
	if err != nil {
		return err
	}

	if err := client.Ping(); err != nil {
		return fmt.Errorf("drand: can't ping the daemon ... %w", err)
	}
	fmt.Fprintf(c.App.Writer, "drand daemon is alive on port %s\n", controlPort(c))
	return nil
}

func remotePingToNode(l log.Logger, addr string) error {
	peer := net.CreatePeer(addr)
	client := net.NewGrpcClient(l)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := client.Home(ctx, peer, &control.HomeRequest{})
	if err != nil {
		return err
	}

	return nil
}

//nolint:gocyclo
func statusCmd(c *cli.Context, l log.Logger) error {
	client, err := controlClient(c, l)
	if err != nil {
		return err
	}

	listIds := c.IsSet(listIdsFlag.Name)
	allIds := c.IsSet(allBeaconsFlag.Name)
	beaconID := c.IsSet(beaconIDFlag.Name)

	if beaconID && (allIds || listIds) {
		return fmt.Errorf("drand: can't use --%s with --%s or --%s flags at the same time",
			beaconIDFlag.Name, allBeaconsFlag.Name, listIdsFlag.Name)
	}

	beaconIDsList := &control.ListBeaconIDsResponse{}
	if allIds || listIds {
		beaconIDsList, err = client.ListBeaconIDs()
		if err != nil {
			return fmt.Errorf("drand: can't get the list of running beacon ids on the daemon ... %w", err)
		}
	} else {
		beaconIDsList.Ids = append(beaconIDsList.Ids, getBeaconID(c))
	}

	if listIds {
		if c.IsSet(jsonFlag.Name) {
			str, err := json.Marshal(beaconIDsList)
			if err != nil {
				return fmt.Errorf("cannot marshal the response ... %w", err)
			}
			fmt.Fprintf(c.App.Writer, "%s \n", string(str))
			return nil
		}

		fmt.Fprintf(c.App.Writer, "running beacon ids on the node: [%s]\n", strings.Join(beaconIDsList.Ids, ", "))
		return nil
	}

	statuses := beaconIDsStatuses{Beacons: make(map[string]*control.StatusResponse)}
	for _, id := range beaconIDsList.Ids {
		resp, err := client.Status(id)
		if err != nil {
			return fmt.Errorf("drand: can't get the status of the network with id [%s]... %w", id, err)
		}

		if c.IsSet(jsonFlag.Name) {
			statuses.Beacons[id] = resp
			continue
		}

		fmt.Fprintf(c.App.Writer, "the status of network with id [%s] is: \n", id)
		fmt.Fprintf(c.App.Writer, "%s \n", core.StatusResponseToString(resp))
	}

	if c.IsSet(jsonFlag.Name) {
		str, err := json.Marshal(statuses)
		if err != nil {
			return fmt.Errorf("cannot marshal the response ... %w", err)
		}
		fmt.Fprintf(c.App.Writer, "%s \n", string(str))
	}

	return nil
}

func schemesCmd(c *cli.Context, l log.Logger) error {
	client, err := controlClient(c, l)
	if err != nil {
		return err
	}

	resp, err := client.ListSchemes()
	if err != nil {
		return fmt.Errorf("drand: can't get the list of scheme ids availables ... %w", err)
	}

	fmt.Fprintf(c.App.Writer, "Drand supports the following list of schemes: \n")

	for i, id := range resp.Ids {
		fmt.Fprintf(c.App.Writer, "%d) %s \n", i, id)
	}

	fmt.Fprintf(c.App.Writer, "\nChoose one of them and set it on --%s flag \n", schemeFlag.Name)
	return nil
}

func showGroupCmd(c *cli.Context, l log.Logger) error {
	client, err := controlClient(c, l)
	if err != nil {
		return err
	}

	beaconID := getBeaconID(c)
	r, err := client.GroupFile(beaconID)
	if err != nil {
		return fmt.Errorf("fetching group file error: %w", err)
	}

	group, err := key.GroupFromProto(r, nil)
	if err != nil {
		return err
	}

	return groupOut(c, group)
}

func showChainInfo(c *cli.Context, l log.Logger) error {
	client, err := controlClient(c, l)
	if err != nil {
		return err
	}

	beaconID := getBeaconID(c)
	resp, err := client.ChainInfo(beaconID)
	if err != nil {
		return fmt.Errorf("could not request chain info: %w", err)
	}

	ci, err := chain.InfoFromProto(resp)
	if err != nil {
		return fmt.Errorf("could not get correct chain info: %w", err)
	}

	if c.Bool(hashOnly.Name) {
		fmt.Fprintf(c.App.Writer, "%s\n", hex.EncodeToString(ci.Hash()))
		return nil
	}
	return printJSON(c.App.Writer, ci.ToProto(nil))
}

func showPublicCmd(c *cli.Context, l log.Logger) error {
	client, err := controlClient(c, l)
	if err != nil {
		return err
	}

	beaconID := getBeaconID(c)
	resp, err := client.PublicKey(beaconID)
	if err != nil {
		return fmt.Errorf("drand: could not request drand.public: %w", err)
	}

	return printJSON(c.App.Writer, resp)
}

func backupDBCmd(c *cli.Context, l log.Logger) error {
	client, err := controlClient(c, l)
	if err != nil {
		return err
	}

	outDir := c.String(backupOutFlag.Name)
	beaconID := getBeaconID(c)
	err = client.BackupDB(outDir, beaconID)
	if err != nil {
		return fmt.Errorf("could not back up: %w", err)
	}

	return nil
}

func controlPort(c *cli.Context) string {
	port := c.String(controlFlag.Name)
	if port == "" {
		port = core.DefaultControlPort
	}
	return port
}

func controlClient(c *cli.Context, l log.Logger) (*net.ControlClient, error) {
	port := controlPort(c)
	client, err := net.NewControlClient(l, port)
	if err != nil {
		return nil, fmt.Errorf("can't instantiate control client: %w", err)
	}
	return client, nil
}

func printJSON(w io.Writer, j interface{}) error {
	buff, err := json.MarshalIndent(j, "", "    ")
	if err != nil {
		return fmt.Errorf("could not JSON marshal: %w", err)
	}
	fmt.Fprintln(w, string(buff))
	return nil
}

//nolint:unused
func selfSign(c *cli.Context, l log.Logger) error {
	conf := contextToConfig(c, l)

	beaconID := getBeaconID(c)

	fs := key.NewFileStore(conf.ConfigFolderMB(), beaconID)
	pair, err := fs.LoadKeyPair()

	if err != nil {
		return fmt.Errorf("beacon id [%s] - loading private/public: %w", beaconID, err)
	}
	if pair.Public.ValidSignature() == nil {
		fmt.Fprintf(c.App.Writer, "beacon id [%s] - public identity already self signed.\n", beaconID)
		return nil
	}

	if err := pair.SelfSign(); err != nil {
		return fmt.Errorf("failed to self-sign keypair for beacon id [%s]: %w", beaconID, err)
	}
	if err := fs.SaveKeyPair(pair); err != nil {
		return fmt.Errorf("beacon id [%s] - saving identity: %w", beaconID, err)
	}

	fmt.Fprintf(c.App.Writer, "beacon id [%s] - Public identity self signed for scheme %s", beaconID, pair.Scheme().Name)
	fmt.Fprintln(c.App.Writer, printJSON(c.App.Writer, pair.Public.TOML()))
	return nil
}

const refreshRate = 500 * time.Millisecond

//nolint:funlen
func checkCmd(c *cli.Context, l log.Logger) error {
	defer l.Infow("Finished sync")

	ctrlClient, err := controlClient(c, l)
	if err != nil {
		return fmt.Errorf("unable to create control client: %w", err)
	}

	addrs := strings.Split(c.String(syncNodeFlag.Name), ",")

	channel, errCh, err := ctrlClient.StartCheckChain(c.Context, c.String(hashInfoReq.Name),
		addrs, uint64(c.Int(upToFlag.Name)), c.String(beaconIDFlag.Name))

	if err != nil {
		l.Errorw("Error checking chain", "err", err)
		return fmt.Errorf("error asking to check chain up to %d: %w", c.Int(upToFlag.Name), err)
	}

	var current uint64
	target := uint64(c.Int(upToFlag.Name))
	s := spinner.New(spinner.CharSets[9], refreshRate)
	s.PreUpdate = func(spin *spinner.Spinner) {
		curr := atomic.LoadUint64(&current)
		spin.Suffix = fmt.Sprintf("  synced round up to %d "+
			"\t- current target %d"+
			"\t--> %.3f %% - "+
			"Waiting on new rounds...", curr, target, 100*float64(curr)/float64(target))
	}
	s.Start()
	defer s.Stop()

	// The following could be much simpler if we don't want to be nice on the user and display comprehensive logs
	// on the client side.
	isCorrecting, success := false, false
	for {
		select {
		case progress, ok := <-channel:
			if !ok {
				// let the spinner time to refresh
				time.Sleep(refreshRate)
				if success {
					// we need an empty line to not clash with the spinner
					fmt.Println()
					l.Infow("Finished correcting faulty beacons, " +
						"we recommend running the same command a second time to confirm all beacons are now valid")
				}
				return nil
			}
			// if we received at least one progress update after switching to correcting
			success = isCorrecting
			if progress.Current == 0 {
				// let the spinner time to refresh
				time.Sleep(refreshRate)
				// we need an empty line to not clash with the spinner
				fmt.Println()
				l.Infow("Finished checking chain validity")
				if progress.Target > 0 {
					l.Warnw("Faulty beacon found!", "amount", progress.Target)
					isCorrecting = true
				} else {
					l.Warnw("No faulty beacon found!")
				}
			}
			atomic.StoreUint64(&current, progress.Current)
			atomic.StoreUint64(&target, progress.Target)
		case err, ok := <-errCh:
			if !ok {
				l.Infow("Error channel was closed")
				return nil
			}
			// note that grpc's "error reading from server: EOF" won't trigger this so we really only catch the case
			// where the server gracefully closed the connection.
			if errors.Is(err, io.EOF) {
				// let the spinner time to refresh
				time.Sleep(refreshRate)
				// make sure to exhaust our progress channel
				progress, ok := <-channel
				if ok {
					if atomic.LoadUint64(&target) > progress.Target {
						// we need an empty line to not clash with the spinner
						fmt.Println()
						l.Infow("Finished checking chain validity")
						l.Warnw("Faulty beacon found!", "amount", progress.Target)
					} else {
						atomic.StoreUint64(&current, progress.Current)
						// let the spinner time to refresh again
						time.Sleep(refreshRate)
						// we need an empty line to not clash with the spinner
						fmt.Println()
					}
				}

				if success {
					// we need an empty line to not clash with the spinner
					fmt.Println()
					l.Infow("Finished correcting faulty beacons, " +
						"we recommend running the same command a second time to confirm all beacons are now valid")
				}

				return nil
			}

			l.Errorw("received an error", "err", err)
			return fmt.Errorf("errror when checking the chain: %w", err)
		}
	}
}

func syncCmd(c *cli.Context, l log.Logger) error {
	if c.Bool(followFlag.Name) {
		return followSync(c, l)
	}

	return checkCmd(c, l)
}

func followSync(c *cli.Context, l log.Logger) error {
	ctrlClient, err := controlClient(c, l)
	if err != nil {
		return fmt.Errorf("unable to create control client: %w", err)
	}
	defer ctrlClient.Close()

	addrs := strings.Split(c.String(syncNodeFlag.Name), ",")
	channel, errCh, err := ctrlClient.StartFollowChain(c.Context, c.String(hashInfoReq.Name),
		addrs, uint64(c.Int(upToFlag.Name)), getBeaconID(c))

	if err != nil {
		return fmt.Errorf("error asking to follow chain: %w", err)
	}

	var current uint64
	var target uint64

	last := time.Now().Unix()

	s := spinner.New(spinner.CharSets[9], refreshRate)
	s.PreUpdate = func(spin *spinner.Spinner) {
		curr := atomic.LoadUint64(&current)
		tar := atomic.LoadUint64(&target)
		dur := time.Now().Unix() - atomic.LoadInt64(&last)

		spin.Suffix = fmt.Sprintf("  synced round up to %d "+
			"- current target %d"+
			"\t--> %.3f %% - "+
			"Last update received %3ds ago. Waiting on new rounds...", curr, tar, 100*float64(curr)/float64(tar), dur)
	}

	s.FinalMSG = "\nSync stopped\n"

	s.Start()
	defer s.Stop()
	for {
		select {
		case progress := <-channel:
			atomic.StoreUint64(&current, progress.Current)
			atomic.StoreUint64(&target, progress.Target)
			atomic.StoreInt64(&last, time.Now().Unix())
		case err := <-errCh:
			if errors.Is(err, io.EOF) {
				// we need a new line because of the spinner
				fmt.Println()
				l.Infow("Finished following beacon chain", "reached", current,
					"server closed stream with", err)
				return nil
			}
			return fmt.Errorf("errror on following the chain: %w", err)
		}
	}
}
