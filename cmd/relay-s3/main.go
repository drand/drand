package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/drand/drand/client"
	dlog "github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/metrics/pprof"
	json "github.com/nikkolasg/hexjson"
	cli "github.com/urfave/cli/v2"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.version=`git describe --tags` -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`"
var (
	version   = "master"
	gitCommit = "none"
	buildDate = "unknown"
	log       = dlog.DefaultLogger
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
			Name:     "bucket",
			Usage:    "name of the AWS bucket to upload to",
			Aliases:  []string{"b"},
			Required: true,
		},
		&cli.StringFlag{
			Name:    "region",
			Usage:   "name of the AWS region to use",
			Aliases: []string{"r"},
		},
		&cli.StringFlag{
			Name:    "grpc-connect",
			Usage:   "host:port to dial to a drand gRPC API",
			Aliases: []string{"connect", "c"},
		},
		&cli.StringSliceFlag{
			Name:    "http-connect",
			Usage:   "URL(s) of drand HTTP API(s) to relay",
			Aliases: []string{"u"},
		},
		&cli.StringFlag{
			Name:  "cert",
			Usage: "file containing GRPC transport credentials of peer",
		},
		&cli.BoolFlag{
			Name:  "insecure",
			Usage: "allow insecure GRPC connection",
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

		sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(cctx.String("region"))}))
		_, err := sess.Config.Credentials.Get() // check credentials have been set
		if err != nil {
			return err
		}

		uploader := s3manager.NewUploader(sess)

		var w client.Watcher

		if len(cctx.StringSlice("http-connect")) > 0 {
			w, err = newHTTPWatcher(cctx.StringSlice("http-connect"))
		} else if cctx.String("grpc-connect") != "" {
			w, err = newGRPCWatcher(
				cctx.String("grpc-connect"),
				cctx.String("cert"),
				cctx.Bool("insecure"),
			)
		} else {
			return errors.New("missing GRPC or HTTP address(es)")
		}
		if err != nil {
			return err
		}

		ctx := context.Background()
		for res := range w.Watch(ctx) {
			go uploadRandomness(ctx, uploader, cctx.String("bucket"), res)
		}
		return nil
	},
}

func newGRPCWatcher(addr string, cert string, insecure bool) (client.Watcher, error) {
	return nil, errors.New("not implemented")
}

func newHTTPWatcher(urls []string) (client.Watcher, error) {
	return client.New(client.WithHTTPEndpoints(urls), client.WithCacheSize(0))
}

func uploadRandomness(ctx context.Context, uploader *s3manager.Uploader, bucket string, result client.Result) {
	for {
		rd, ok := result.(*client.RandomData)
		if !ok {
			log.Error("relay_s3", "unexpected underlying result type")
			return
		}
		data, err := json.Marshal(rd)
		if err != nil {
			log.Error("relay_s3", "failed to marshal randomness", "error", err)
			return
		}
		r, err := uploader.UploadWithContext(ctx, &s3manager.UploadInput{
			Bucket:          aws.String(bucket),
			Key:             aws.String(fmt.Sprintf("public/%v", result.Round())),
			Body:            bytes.NewBuffer(data),
			ContentEncoding: aws.String("application/json"),
			CacheControl:    aws.String("immutable"),
		})
		if err != nil {
			log.Error("relay_s3", "failed to upload randomness", "round", result.Round(), "error", err)
			t := time.NewTimer(time.Minute)
			select {
			case <-t.C:
				continue
			case <-ctx.Done():
				t.Stop()
				return
			}
		}
		log.Info("relay_s3", "uploaded randmoness", "round", result.Round(), "upload_output", r)
		return
	}
	// TODO: upload to /public/latest
}
