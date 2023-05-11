package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/gorilla/handlers"
	grpcprometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/urfave/cli/v2"

	common2 "github.com/drand/drand/common"
	"github.com/drand/drand/common/log"
	dhttp "github.com/drand/drand/internal/http"
	"github.com/drand/drand/internal/lib"
	"github.com/drand/drand/internal/metrics"
	"github.com/drand/drand/internal/metrics/pprof"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`"
var (
	gitCommit = "none"
	buildDate = "unknown"
)

const accessLogPermFolder = 0o666

var accessLogFlag = &cli.StringFlag{
	Name:    "access-log",
	Usage:   "file to log http accesses to",
	EnvVars: []string{"DRAND_RELAY_ACCESS_LOG"},
}

var listenFlag = &cli.StringFlag{
	Name:    "bind",
	Usage:   "local host:port to bind the listener",
	EnvVars: []string{"DRAND_RELAY_BIND"},
}

var metricsFlag = &cli.StringFlag{
	Name:    "metrics",
	Usage:   "local host:port to bind a metrics servlet (optional)",
	EnvVars: []string{"DRAND_RELAY_METRICS"},
}

var tracesFlag = &cli.StringFlag{
	Name:    "traces",
	Usage:   "Publish metrics to the specific OpenTelemetry compatible host:port server. E.g. 127.0.0.1:4317",
	EnvVars: []string{"DRAND_TRACES"},
}

var tracesProbabilityFlag = &cli.Float64Flag{
	Name:    "traces-probability",
	Usage:   "Publish metrics to the specific OpenTelemetry compatible host:port server.",
	EnvVars: []string{"DRAND_TRACES_PROBABILITY"},
	Value:   0.05,
}

// Relay a GRPC connection to an HTTP server.
//
//nolint:gocyclo,funlen
func Relay(c *cli.Context) error {
	tracesProbability := 0.1
	if c.IsSet(tracesProbabilityFlag.Name) {
		tracesProbability = c.Float64(tracesProbabilityFlag.Name)
	}

	_, tracerShutdown := metrics.InitTracer("drand_relay", c.String(tracesFlag.Name), tracesProbability)
	defer tracerShutdown(c.Context)

	cliLog := log.FromContextOrDefault(c.Context)
	version := common2.GetAppVersion()

	if c.IsSet(metricsFlag.Name) {
		metricsListener := metrics.Start(cliLog, c.String(metricsFlag.Name), pprof.WithProfile(), nil)
		defer metricsListener.Close()

		if err := metrics.PrivateMetrics.Register(grpcprometheus.DefaultClientMetrics); err != nil {
			return err
		}
	}

	hashFlagSet := c.IsSet(lib.HashFlag.Name)
	if hashFlagSet {
		return fmt.Errorf("--%s is deprecated on relay http, please use %s instead", lib.HashFlag.Name, lib.HashListFlag.Name)
	}

	relayCtx := log.ToContext(c.Context, cliLog.Named("relay"))
	handler, err := dhttp.New(relayCtx, fmt.Sprintf("drand/%s (%s)",
		version, gitCommit))
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
		hashesMap[common2.DefaultChainHash] = true
	}

	skipHashes := make(map[string]bool)
	for hash := range hashesMap {
		// todo: don't reuse 'c'
		if hash != common2.DefaultChainHash {
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
			cliLog.Warnw("failed to create client", "hash", hash, "error", err)
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

	bind := "127.0.0.1:0"
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
		if hash != common2.DefaultChainHash {
			req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/%s/public/0", hash), http.NoBody)
		}

		rr := httptest.NewRecorder()
		handler.GetHTTPHandler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			cliLog.Warnw("", "binary", "relay", "chain-hash", hash, "startup failed", rr.Code)
		}
	}

	fmt.Printf("Listening at %s\n", listener.Addr())
	// http.Serve is marked as problematic because it does not
	// have tweaked timeouts out of the box.

	//nolint
	return http.Serve(listener, handler.GetHTTPHandler())
}

func main() {
	version := common2.GetAppVersion()
	lg := log.New(nil, log.DefaultLevel, false)

	app := &cli.App{
		Name:    "relay",
		Version: version.String(),
		Usage:   "Relay a Drand group to a public HTTP Rest API",
		Flags:   append(lib.ClientFlags, lib.HashListFlag, listenFlag, accessLogFlag, metricsFlag, tracesFlag, tracesProbabilityFlag),
		Action: func(ctx *cli.Context) error {
			ctx.Context = log.ToContext(ctx.Context, lg)
			return Relay(ctx)
		},
	}

	// See https://cli.urfave.org/v2/examples/bash-completions/#enabling for how to turn on.
	app.EnableBashCompletion = true

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("drand HTTP relay %v (date %v, commit %v)\n", version, buildDate, gitCommit)
	}

	err := app.Run(os.Args)
	if err != nil {
		lg.Fatalw("", "binary", "relay", "err", err)
	}
}
