package drand

import (
	"fmt"

	"github.com/drand/drand/core"
	"github.com/drand/drand/key"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/metrics/pprof"
	"github.com/urfave/cli/v2"
)

func startCmd(c *cli.Context) error {
	conf := contextToConfig(c)

	// Create and start drand daemon
	drandDaemon, err := core.NewDrandDaemon(conf)
	if err != nil {
		return fmt.Errorf("can't instantiate drand daemon %s", err)
	}

	// Load possible existing stores
	stores, err := key.NewFileStores(conf.ConfigFolderMB())
	if err != nil {
		return err
	}

	for beaconID, fs := range stores {
		var bp *core.BeaconProcess

		_, errG := fs.LoadGroup()
		_, errS := fs.LoadShare()

		// XXX place that logic inside core/ directly with only one method
		freshRun := errG != nil || errS != nil

		if freshRun {
			fmt.Printf("beacon id [%s]: will run as fresh install -> expect to run DKG.\n", beaconID)
			bp, err = drandDaemon.AddNewBeaconProcess(beaconID, fs)
			if err != nil {
				fmt.Printf("beacon id [%s]: can't instantiate randomness beacon. err: %s \n", beaconID, err)
			}
		} else {
			fmt.Printf("beacon id [%s]: will already start running randomness beacon.\n", beaconID)
			bp, err = drandDaemon.AddNewBeaconProcess(beaconID, fs)
			if err != nil {
				fmt.Printf("beacon id [%s]: can't instantiate randomness beacon. err: %s \n", beaconID, err)
			}

			if _, err := bp.Load(); err != nil {
				return err
			}

			// XXX make it configurable so that new share holder can still start if
			// nobody started.
			// drand.StartBeacon(!c.Bool(pushFlag.Name))
			catchup := true
			bp.StartBeacon(catchup)
		}

		// Start metrics server
		if c.IsSet(metricsFlag.Name) {
			_ = metrics.Start(c.String(metricsFlag.Name), pprof.WithProfile(), bp.PeerMetrics)
		}
	}

	<-drandDaemon.WaitExit()

	return nil
}

func stopDaemon(c *cli.Context) error {
	ctrlClient, err := controlClient(c)
	if err != nil {
		return err
	}
	if _, err := ctrlClient.Shutdown(); err != nil {
		return fmt.Errorf("error stopping drand daemon: %w", err)
	}
	fmt.Println("drand daemon stopped correctly. Bye.")
	return nil
}
