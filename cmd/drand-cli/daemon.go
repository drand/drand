package drand

import (
	"fmt"
	"github.com/drand/drand/core"
	"github.com/urfave/cli/v2"
)

func startCmd(c *cli.Context) error {
	conf := contextToConfig(c)

	// Create and start drand daemon
	drandDaemon, err := core.NewDrandDaemon(conf)
	if err != nil {
		return fmt.Errorf("can't instantiate drand daemon %s", err)
	}

	// Check stores and start BeaconProcess
	err = drandDaemon.LoadBeacons(c.String(metricsFlag.Name))
	if err != nil {
		return fmt.Errorf("couldn't load existing beacons: %s", err)
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
