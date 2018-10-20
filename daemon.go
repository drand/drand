package main

import (
	"runtime"

	"github.com/dedis/drand/core"
	"github.com/dedis/drand/key"
	"github.com/nikkolasg/slog"
	"github.com/urfave/cli"
)

func startDaemon(c *cli.Context) error {
	if runtime.GOOS == "windows" && !c.Bool("insecure") {
		slog.Fatal("TLS is not available on Windows, please run in insecure mode")
	}

	conf := contextToConfig(c)
	fs := key.NewFileStore(conf.ConfigFolder())
	var drand *core.Drand

	// determine if we already ran a DKG or not
	_, errG := fs.LoadGroup()
	_, errS := fs.LoadShare()
	_, errD := fs.LoadDistPublic()
	// XXX place that logic inside core/ directly with only one method
	freshRun := errG != nil || errS != nil || errD != nil
	var err error
	if freshRun {
		slog.Infof("drand: will run as fresh install -> expect to run DKG.")
		drand, err = core.NewDrand(fs, conf)
		if err != nil {
			slog.Fatalf("drand: can't instantiate drand instance %s", err)
		}
		// wait indefinitely  - XXX analyzes goroutine graphs to see if it makes
		// sense in practice
		runtime.Goexit()
	} else {
		drand, err = core.LoadDrand(fs, conf)
		if err != nil {
			slog.Fatalf("drand: can't load drand instance %s", err)
		}
		drand.StartBeacon()
	}
	return nil
}

func stopDaemon(c *cli.Context) error {
	// XXX TODO
	panic("not implemented yet")
}
