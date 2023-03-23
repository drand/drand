package drand

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/drand/drand/core"
	"github.com/drand/drand/log"
)

func startCmd(c *cli.Context, l log.Logger) error {
	conf := contextToConfig(c, l)

	// Create and start drand daemon
	drandDaemon, err := core.NewDrandDaemon(conf)
	if err != nil {
		return fmt.Errorf("can't instantiate drand daemon %w", err)
	}

	singleBeacon := false
	if c.IsSet(beaconIDFlag.Name) {
		singleBeacon = true
	}

	// Check stores and start BeaconProcess
	err = drandDaemon.LoadBeaconsFromDisk(c.String(metricsFlag.Name), singleBeacon, c.String(beaconIDFlag.Name))
	if err != nil {
		return fmt.Errorf("couldn't load existing beacons: %w", err)
	}

	<-drandDaemon.WaitExit()
	return nil
}

func stopDaemon(c *cli.Context, lg log.Logger) error {
	ctrlClient, err := controlClient(c, lg)
	if err != nil {
		return err
	}

	isBeaconIDSet := c.IsSet(beaconIDFlag.Name)
	if isBeaconIDSet {
		beaconID := getBeaconID(c)
		_, err = ctrlClient.Shutdown(beaconID)

		if err != nil {
			return fmt.Errorf("error stopping beacon process [%s]: %w", beaconID, err)
		}
		fmt.Fprintf(c.App.Writer, "beacon process [%s] stopped correctly. Bye.\n", beaconID)
	} else {
		_, err = ctrlClient.Shutdown("")

		if err != nil {
			return fmt.Errorf("error stopping drand daemon: %w", err)
		}
		fmt.Fprintf(c.App.Writer, "drand daemon stopped correctly. Bye.\n")
	}

	return nil
}
