package main

import (
	"bytes"
	"context"
	"encoding/hex"
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

const (
	uploadRetryInterval = time.Minute
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
		Name:     "drand-relay-s3",
		Version:  version,
		Usage:    "AWS S3 relay for randomness beacon",
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
	Name:  "run",
	Usage: "start a drand AWS S3 relay process",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "chain-hash",
			Usage:   "hex encoded hash of the drand group chain (optional)",
			Aliases: []string{"chain"},
		},
		&cli.StringFlag{
			Name:     "bucket",
			Usage:    "name of the AWS bucket to upload to",
			Aliases:  []string{"b"},
			Required: true,
		},
		&cli.StringFlag{
			Name:    "region",
			Usage:   "name of the AWS region to use (optional)",
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
			Usage: "file containing GRPC transport credentials of peer (optional)",
		},
		&cli.BoolFlag{
			Name:  "insecure",
			Usage: "allow insecure GRPC connection (optional)",
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

		var err error
		var chainHash []byte

		if cctx.String("chain-hash") != "" {
			chainHash, err = hex.DecodeString(cctx.String("chain-hash"))
			if err != nil {
				return fmt.Errorf("decoding chain hash: %w", err)
			}
		}

		sess, err := session.NewSession(&aws.Config{Region: aws.String(cctx.String("region"))})
		if err != nil {
			return fmt.Errorf("creating aws session: %w", err)
		}

		_, err = sess.Config.Credentials.Get()
		if err != nil {
			return fmt.Errorf("checking credentials: %w", err)
		}

		uploader := s3manager.NewUploader(sess)

		var w client.Watcher

		if len(cctx.StringSlice("http-connect")) > 0 {
			w, err = newHTTPWatcher(cctx.StringSlice("http-connect"), chainHash)
		} else if cctx.String("grpc-connect") != "" {
			w, err = newGRPCWatcher(cctx.String("grpc-connect"), cctx.String("cert"), cctx.Bool("insecure"))
		} else {
			return errors.New("missing GRPC or HTTP address(es)")
		}
		if err != nil {
			return err
		}

		ctx := context.Background()
		for res := range w.Watch(ctx) {
			log.Info("relay_s3", "got randomness", "round", res.Round())
			go uploadRandomness(ctx, uploader, cctx.String("bucket"), res)
		}
		return nil
	},
}

func newGRPCWatcher(addr string, cert string, insecure bool) (client.Watcher, error) {
	return nil, errors.New("not implemented")
}

func newHTTPWatcher(urls []string, chainHash []byte) (client.Watcher, error) {
	if chainHash == nil {
		return client.New(client.WithInsecureHTTPEndpoints(urls), client.WithCacheSize(0))
	}
	return client.New(client.WithChainHash(chainHash), client.WithHTTPEndpoints(urls), client.WithCacheSize(0))
}

func uploadRandomness(ctx context.Context, uploader *s3manager.Uploader, bucket string, result client.Result) {
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
	for {
		r, err := uploader.UploadWithContext(ctx, &s3manager.UploadInput{
			ACL:          aws.String("public-read"),
			Bucket:       aws.String(bucket),
			Key:          aws.String(fmt.Sprintf("public/%v", result.Round())),
			Body:         bytes.NewBuffer(data),
			ContentType:  aws.String("application/json"),
			CacheControl: aws.String("immutable"),
		})
		if err != nil {
			log.Error("relay_s3", "failed to upload randomness", "round", result.Round(), "error", err)
			t := time.NewTimer(uploadRetryInterval)
			select {
			case <-t.C:
				continue
			case <-ctx.Done():
				t.Stop()
				return
			}
		}
		log.Info("relay_s3", "uploaded randomness", "round", result.Round(), "location", r.Location)
		break
	}
	// TODO: upload to /public/latest
}
