package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	dhttp "github.com/drand/drand/http"
	drand "github.com/drand/drand/protobuf/drand"

	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var listenFlag = &cli.StringFlag{
	Name:  "bind",
	Usage: "local host:port to bind the listener",
}

var connectFlag = &cli.StringFlag{
	Name:  "connect",
	Usage: "host:port to dial to a GRPC drand public API",
}

var certFlag = &cli.StringFlag{
	Name:  "cert",
	Usage: "file containing GRPC transport credentials of peer",
}

// Relay a GRPC connection to an HTTP server.
func Relay(c *cli.Context) error {
	if !c.IsSet(connectFlag.Name) {
		return fmt.Errorf("A 'connect' host must be provided")
	}

	opts := []grpc.DialOption{}
	if c.IsSet(certFlag.Name) {
		creds, _ := credentials.NewClientTLSFromFile(c.String(certFlag.Name), "")
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithInsecure())
	}
	conn, err := grpc.Dial(c.String(connectFlag.Name), opts...)
	if err != nil {
		return fmt.Errorf("Failed to connect to group member: %w", err)
	}

	client := drand.NewPublicClient(conn)

	handler, err := dhttp.New(c.Context, client)
	if err != nil {
		return fmt.Errorf("Failed to create rest handler: %w", err)
	}

	bind := ":0"
	if c.IsSet(listenFlag.Name) {
		bind = c.String(listenFlag.Name)
	}
	http.ListenAndServe(bind, handler)

	return nil
}

func main() {
	app := &cli.App{
		Name:   "relay",
		Usage:  "Relay a Drand group to a public HTTP Rest API",
		Flags:  []cli.Flag{listenFlag, connectFlag, certFlag},
		Action: Relay,
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
