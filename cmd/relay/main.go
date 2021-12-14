package main

import (
	"encoding/hex"
	"fmt"
	client2 "github.com/drand/drand/client"
	"net"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/drand/drand/cmd/client/lib"
	"github.com/drand/drand/common"
	dhttp "github.com/drand/drand/http"
	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/metrics/pprof"

	"github.com/gorilla/handlers"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/urfave/cli/v2"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`"
var (
	gitCommit = "none"
	buildDate = "unknown"
)

const accessLogPermFolder = 0666

var accessLogFlag = &cli.StringFlag{
	Name:  "access-log",
	Usage: "file to log http accesses to",
}

var listenFlag = &cli.StringFlag{
	Name:  "bind",
	Usage: "local host:port to bind the listener",
}

var metricsFlag = &cli.StringFlag{
	Name:  "metrics",
	Usage: "local host:port to bind a metrics servlet (optional)",
}

// Relay a GRPC connection to an HTTP server.
func Relay(c *cli.Context) error {
	version := common.GetAppVersion()

	if c.IsSet(metricsFlag.Name) {
		metricsListener := metrics.Start(c.String(metricsFlag.Name), pprof.WithProfile(), nil)
		defer metricsListener.Close()

		if err := metrics.PrivateMetrics.Register(grpc_prometheus.DefaultClientMetrics); err != nil {
			return err
		}
	}

	client, err := lib.Create(c, c.IsSet(metricsFlag.Name))
	if err != nil {
		return err
	}

	handler, err := dhttp.New(c.Context, client, fmt.Sprintf("drand/%s (%s)", version, gitCommit), log.DefaultLogger().With("binary", "relay"))
	if err != nil {
		return fmt.Errorf("failed to create rest handler: %w", err)
	}

	if c.IsSet(lib.HashFlag.Name) {
		hash, err := hex.DecodeString(c.String(lib.HashFlag.Name))
		if err != nil {
			return fmt.Errorf("failed to decode hash flag: %w", err)
		}
		handler.HandlerDrand.CreateBeaconHandler(client, string(hash))
	} else {
		if c.IsSet(lib.HashListFlag.Name) {
			hashList := c.StringSlice(lib.HashListFlag.Name)
			for _, hashHex := range hashList {
				hash, err := hex.DecodeString(hashHex)
				if err != nil {
					return fmt.Errorf("failed to decode hash flag: %w", err)
				}

				c, err := lib.Create(c, c.IsSet(metricsFlag.Name), client2.WithChainHash(hash))
				if err != nil {
					return err
				}

				handler.HandlerDrand.CreateBeaconHandler(c, fmt.Sprintf("%x", hash))
			}
		} else {
			return fmt.Errorf("must specify flag %s or %s", lib.HashFlag.Name, lib.HashListFlag.Name)
		}
	}

	if c.IsSet(accessLogFlag.Name) {
		logFile, err := os.OpenFile(c.String(accessLogFlag.Name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, accessLogPermFolder)
		if err != nil {
			return fmt.Errorf("failed to open access log: %w", err)
		}
		defer logFile.Close()
		handler.HandlerHTTP = handlers.CombinedLoggingHandler(logFile, handler.HandlerHTTP)
	} else {
		handler.HandlerHTTP = handlers.CombinedLoggingHandler(os.Stdout, handler.HandlerHTTP)
	}

	bind := "localhost:0"
	if c.IsSet(listenFlag.Name) {
		bind = c.String(listenFlag.Name)
	}
	listener, err := net.Listen("tcp", bind)
	if err != nil {
		return err
	}

	// jumpstart bootup
	req, _ := http.NewRequest("GET", "/public/0", http.NoBody)
	rr := httptest.NewRecorder()
	handler.HandlerHTTP.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		log.DefaultLogger().Warnw("", "binary", "relay", "startup failed", rr.Code)
	}

	fmt.Printf("Listening at %s\n", listener.Addr())
	return http.Serve(listener, handler.HandlerHTTP)
}

func main() {
	version := common.GetAppVersion()

	app := &cli.App{
		Name:    "relay",
		Version: version.String(),
		Usage:   "Relay a Drand group to a public HTTP Rest API",
		Flags:   append(lib.ClientFlags, lib.HashListFlag, listenFlag, accessLogFlag, metricsFlag),
		Action:  Relay,
	}
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("drand HTTP relay %v (date %v, commit %v)\n", version, buildDate, gitCommit)
	}

	err := app.Run(os.Args)
	if err != nil {
		log.DefaultLogger().Fatalw("", "binary", "relay", "err", err)
	}
}
