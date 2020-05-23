package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"

	dhttp "github.com/drand/drand/http"
	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/metrics/pprof"
	drand "github.com/drand/drand/protobuf/drand"

	"github.com/gorilla/handlers"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.version=`git describe --tags` -X main.gitCommit=`git rev-parse HEAD`"
var (
	version   = "master"
	gitCommit = "none"
)

var accessLogFlag = &cli.StringFlag{
	Name:  "access-log",
	Usage: "file to log http accesses to",
}

var connectFlag = &cli.StringFlag{
	Name:  "connect",
	Usage: "host:port to dial to a GRPC drand public API",
}

var certFlag = &cli.StringFlag{
	Name:  "cert",
	Usage: "file containing GRPC transport credentials of peer",
}

var insecureFlag = &cli.BoolFlag{
	Name:  "insecure",
	Usage: "Allow non-tls connections to GRPC server",
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
	if !c.IsSet(connectFlag.Name) {
		return fmt.Errorf("A 'connect' host must be provided")
	}

	if c.IsSet(metricsFlag.Name) {
		go metrics.Start(c.String(metricsFlag.Name), pprof.WithProfile())
	}

	opts := []grpc.DialOption{}
	if c.IsSet(certFlag.Name) {
		creds, _ := credentials.NewClientTLSFromFile(c.String(certFlag.Name), "")
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else if c.Bool(insecureFlag.Name) {
		opts = append(opts, grpc.WithInsecure())
	} else {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	}
	conn, err := grpc.Dial(c.String(connectFlag.Name), opts...)
	if err != nil {
		return fmt.Errorf("Failed to connect to group member: %w", err)
	}

	client := drand.NewPublicClient(conn)

	handler, err := dhttp.New(c.Context, client, fmt.Sprintf("drand/%s (%s)", version, gitCommit), log.DefaultLogger.With("binary", "relay"))
	if err != nil {
		return fmt.Errorf("Failed to create rest handler: %w", err)
	}

	if c.IsSet(accessLogFlag.Name) {
		logFile, err := os.OpenFile(c.String(accessLogFlag.Name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
		if err != nil {
			return fmt.Errorf("Failed to open access log: %w", err)
		}
		defer logFile.Close()
		handler = handlers.CombinedLoggingHandler(logFile, handler)
	} else {
		handler = handlers.CombinedLoggingHandler(os.Stdout, handler)
	}

	bind := ":0"
	if c.IsSet(listenFlag.Name) {
		bind = c.String(listenFlag.Name)
	}
	listener, err := net.Listen("tcp", bind)
	if err != nil {
		return err
	}
	fmt.Printf("Listening at %s\n", listener.Addr())
	return http.Serve(listener, handler)
}

func main() {
	app := &cli.App{
		Name:   "relay",
		Usage:  "Relay a Drand group to a public HTTP Rest API",
		Flags:  []cli.Flag{listenFlag, connectFlag, certFlag, insecureFlag, accessLogFlag, metricsFlag},
		Action: Relay,
	}

	err := app.Run(os.Args)
	if err != nil {
		log.DefaultLogger.Fatal("binary", "relay", "err", err)
	}
}
