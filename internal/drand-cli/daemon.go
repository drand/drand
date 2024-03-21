package drand

import (
	"fmt"

	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/attribute"

	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/common/tracer"
	"github.com/drand/drand/v2/internal/core"
)

func startCmd(c *cli.Context, l log.Logger) error {
	conf := contextToConfig(c, l)
	ctx := c.Context

	trace, tracerShutdown := tracer.InitTracer("drand", conf.TracesEndpoint(), conf.TracesProbability())
	defer tracerShutdown(ctx)

	ctx, span := trace.Start(ctx, "startCmd")

	// Create and start drand daemon
	drandDaemon, err := core.NewDrandDaemon(ctx, conf)
	if err != nil {
		err = fmt.Errorf("can't instantiate drand daemon %w", err)
		span.RecordError(err)
		span.End()
		return err
	}

	singleBeacon := false
	if c.IsSet(beaconIDFlag.Name) {
		singleBeacon = true
	}
	span.SetAttributes(
		attribute.Bool("singleBeaconMode", singleBeacon),
	)

	// Check stores and start BeaconProcess
	err = drandDaemon.LoadBeaconsFromDisk(ctx, c.String(metricsFlag.Name), singleBeacon, c.String(beaconIDFlag.Name))
	if err != nil {
		err = fmt.Errorf("couldn't load existing beacons: %w", err)
		span.RecordError(err)
		span.End()
		return err
	}

	span.End()
	<-drandDaemon.WaitExit()
	return nil
}

func stopDaemon(c *cli.Context, lg log.Logger) error {
	ctrlClient, err := controlClient(c, lg)
	if err != nil {
		return err
	}
	defer ctrlClient.Close()

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
