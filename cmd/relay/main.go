package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/gorilla/handlers"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/urfave/cli/v2"

	"github.com/drand/drand/cmd/client/lib"
	"github.com/drand/drand/common"
	dhttp "github.com/drand/drand/http"
	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/metrics/pprof"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`"
var (
	gitCommit = "none"
	buildDate = "unknown"
)

const accessLogPermFolder = 0o666

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
//
//nolint:gocyclo,funlen
func Relay(c *cli.Context) error {
	version := common.GetAppVersion()

	if c.IsSet(metricsFlag.Name) {
		metricsListener := metrics.Start(c.String(metricsFlag.Name), pprof.WithProfile(), nil)
		defer metricsListener.Close()

		if err := metrics.PrivateMetrics.Register(grpc_prometheus.DefaultClientMetrics); err != nil {
			return err
		}
	}

	hashFlagSet := c.IsSet(lib.HashFlag.Name)
	if hashFlagSet {
		return fmt.Errorf("--%s is deprecated on relay http, please use %s instead", lib.HashFlag.Name, lib.HashListFlag.Name)
	}

	handler, err := dhttp.New(c.Context, fmt.Sprintf("drand/%s (%s)",
		version, gitCommit), log.DefaultLogger().Named("relay"))
	if err != nil {
		return fmt.Errorf("failed to create rest handler: %w", err)
	}

	hashesMap := make(map[string]bool)
	if c.IsSet(lib.HashListFlag.Name) {
		hashesList := c.StringSlice(lib.HashListFlag.Name)
		for _, hash := range hashesList {
			hashesMap[hash] = true
		}
	} else {
		hashesMap[common.DefaultChainHash] = true
	}

	skipHashes := make(map[string]bool)
	for hash := range hashesMap {
		// todo: don't reuse 'c'
		if hash != common.DefaultChainHash {
			if _, err := hex.DecodeString(hash); err != nil {
				return fmt.Errorf("failed to decode chain hash value: %w", err)
			}
			if err := c.Set(lib.HashFlag.Name, hash); err != nil {
				return fmt.Errorf("failed to initiate chain hash handler: %w", err)
			}
		} else {
			if err := c.Set(lib.HashFlag.Name, ""); err != nil {
				return fmt.Errorf("failed to initiate default chain hash handler: %w", err)
			}
		}

		subCli, err := lib.Create(c, c.IsSet(metricsFlag.Name))
		if err != nil {
			log.DefaultLogger().Warnw("failed to create client", "hash", hash, "error", err)
			skipHashes[hash] = true
			continue
		}

		handler.RegisterNewBeaconHandler(subCli, hash)
	}

	if len(skipHashes) == len(hashesMap) {
		return fmt.Errorf("failed to create any beacon handlers")
	}

	if c.IsSet(accessLogFlag.Name) {
		logFile, err := os.OpenFile(c.String(accessLogFlag.Name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, accessLogPermFolder)
		if err != nil {
			return fmt.Errorf("failed to open access log: %w", err)
		}
		defer logFile.Close()
		handler.SetHTTPHandler(handlers.CombinedLoggingHandler(logFile, handler.GetHTTPHandler()))
	} else {
		handler.SetHTTPHandler(handlers.CombinedLoggingHandler(os.Stdout, handler.GetHTTPHandler()))
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
	for hash := range hashesMap {
		if skipHashes[hash] {
			continue
		}

		req, _ := http.NewRequest(http.MethodGet, "/public/0", http.NoBody)
		if hash != common.DefaultChainHash {
			req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/%s/public/0", hash), http.NoBody)
		}

		rr := httptest.NewRecorder()
		handler.GetHTTPHandler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			log.DefaultLogger().Warnw("", "binary", "relay", "chain-hash", hash, "startup failed", rr.Code)
		}
	}

	fmt.Printf("Listening at %s\n", listener.Addr())
	return http.Serve(listener, handler.GetHTTPHandler())
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
