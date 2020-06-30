package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/drand/drand/cmd/client/lib"
	dhttp "github.com/drand/drand/http"
	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/metrics/pprof"

	"github.com/gorilla/handlers"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/urfave/cli/v2"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.version=`git describe --tags`
//   -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`"
var (
	version   = "master"
	gitCommit = "none"
	buildDate = "unknown"
)

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

	if c.IsSet(accessLogFlag.Name) {
		logFile, err := os.OpenFile(c.String(accessLogFlag.Name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
		if err != nil {
			return fmt.Errorf("failed to open access log: %w", err)
		}
		defer logFile.Close()
		handler = handlers.CombinedLoggingHandler(logFile, handler)
	} else {
		handler = handlers.CombinedLoggingHandler(os.Stdout, handler)
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
	req, _ := http.NewRequest("GET", "/public/0", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		log.DefaultLogger().Warn("binary", "relay", "startup failed", rr.Code)
	}

	fmt.Printf("Listening at %s\n", listener.Addr())
	return http.Serve(listener, handler)
}

func main() {
	app := &cli.App{
		Name:    "relay",
		Version: version,
		Usage:   "Relay a Drand group to a public HTTP Rest API",
		Flags:   append(lib.ClientFlags, listenFlag, accessLogFlag, metricsFlag),
		Action:  Relay,
	}
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("drand HTTP relay %v (date %v, commit %v)\n", version, buildDate, gitCommit)
	}

	err := app.Run(os.Args)
	if err != nil {
		log.DefaultLogger().Fatal("binary", "relay", "err", err)
	}
}
