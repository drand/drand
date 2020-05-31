package main

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/metrics/pprof"
	cli "github.com/urfave/cli/v2"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.version=`git describe --tags` -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`"
var (
	version   = "master"
	gitCommit = "none"
	buildDate = "unknown"
)

func main() {
	app := &cli.App{
		Name:    "drand-relay-s3",
		Version: version,
		Usage:   "AWS S3 relay for randomness beacon",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "chain-hash",
				Required: true,
				Aliases:  []string{"h"},
			},
		},
		Commands: []*cli.Command{runCmd},
	}
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("drand AWS S3 relay %v (date %v, commit %v)\n", version, buildDate, gitCommit)
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("error: %+v\n", err)
		os.Exit(1)
	}
}

var runCmd = &cli.Command{
	Name: "run",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "grpc-connect",
			Usage: "host:port to dial to a drand gRPC API",
		},
		&cli.StringSliceFlag{
			Name:  "http-connect",
			Usage: "URL(s) of drand HTTP API(s) to relay",
		},
		&cli.StringFlag{
			Name:  "cert",
			Usage: "file containing GRPC transport credentials of peer",
		},
		&cli.BoolFlag{
			Name:  "insecure",
			Usage: "allow insecure connection",
		},
		&cli.StringFlag{
			Name:  "metrics",
			Usage: "local host:port to bind a metrics servlet (optional)",
		},
	},

	Action: func(cctx *cli.Context) error {
		if cctx.IsSet("metrics") {
			metricsListener := metrics.Start(cctx.String("metrics"), pprof.WithProfile(), nil)
			defer metricsListener.Close()
		}

		sess := session.Must(session.NewSession())
		uploader := s3manager.NewUploader(sess)

		f, err := os.Open(filename)
		if err != nil {
			return fmt.Errorf("failed to open file %q, %v", filename, err)
		}

		// Upload the file to S3.
		result, err := uploader.Upload(&s3manager.UploadInput{
			Bucket: aws.String(myBucket),
			Key:    aws.String(myString),
			Body:   f,
		})
		if err != nil {
			return fmt.Errorf("failed to upload file, %v", err)
		}
		fmt.Printf("file uploaded to, %s\n", aws.StringValue(result.Location))
	},
}

func workRelay() {

}
