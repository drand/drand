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
	fs := key.NewFileStore(conf.ConfigFolder())
	var drand *core.Drand
	// determine if we already ran a DKG or not
	_, errG := fs.LoadGroup()
	_, errS := fs.LoadShare()
	// XXX place that logic inside core/ directly with only one method
	freshRun := errG != nil || errS != nil
	var err error
	if freshRun {
		fmt.Println("drand: will run as fresh install -> expect to run DKG.")
		drand, err = core.NewDrand(fs, conf)
		if err != nil {
			return fmt.Errorf("can't instantiate drand instance %s", err)
		}
	} else {
		fmt.Println("drand: will already start running randomness beacon")
		drand, err = core.LoadDrand(fs, conf)
		if err != nil {
			return fmt.Errorf("can't load drand instance %s", err)
		}
		// XXX make it configurable so that new share holder can still start if
		// nobody started.
		// drand.StartBeacon(!c.Bool(pushFlag.Name))
		catchup := true
		drand.StartBeacon(catchup)
	}
	// Start metrics server
	if c.IsSet(metricsFlag.Name) {
		_ = metrics.Start(c.String(metricsFlag.Name), pprof.WithProfile(), drand.PeerMetrics)
	}
	<-drand.WaitExit()

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
