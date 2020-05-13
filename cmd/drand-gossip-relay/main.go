package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"os"
	"time"

	"github.com/drand/drand/cmd/drand-gossip-relay/client"
	"github.com/drand/drand/cmd/drand-gossip-relay/lp2p"
	"github.com/drand/drand/protobuf/drand"
	"github.com/golang/protobuf/proto"
	"github.com/ipfs/go-datastore"
	bds "github.com/ipfs/go-ds-badger2"
	logging "github.com/ipfs/go-log/v2"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	peer "github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	ma "github.com/multiformats/go-multiaddr"
	cli "github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	log = logging.Logger("beacon-relay")
)

func main() {
	logging.SetLogLevel("*", "info")
	logging.SetLogLevel("beacon-relay", "info")

	app := &cli.App{
		Name:        "beacon-relay",
		Version:     "0.0.1",
		Description: "pubsub relay for randomness beacon",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "network-name",
				Aliases: []string{"nn"},
			},
		},
		Commands: []*cli.Command{runCmd, clientCmd},
	}
	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("error: %+v\n", err)
		os.Exit(1)
	}
}
func parseMultiaddrSlice(peer []string) ([]ma.Multiaddr, error) {
	out := make([]ma.Multiaddr, len(peer))
	for i, peer := range peer {
		m, err := ma.NewMultiaddr(peer)
		if err != nil {
			return nil, xerrors.Errorf("parsing multiaddr\"%s\": %w", peer, err)
		}
		out[i] = m
	}
	return out, nil
}

var peerWithFlag = &cli.StringSliceFlag{
	Name:  "peer-with",
	Usage: "list of peers to connect with",
}

var runCmd = &cli.Command{
	Name: "run",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "connect",
			Usage: "host:port to dial to a drand gRPC PI",
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
			Name:  "listen",
			Usage: "listen addr for libp2p",
			Value: "/ip4/0.0.0.0/tcp/44544",
		},
		peerWithFlag, idFlag,
	},

	Action: func(cctx *cli.Context) error {
		bootstrap, err := parseMultiaddrSlice(cctx.StringSlice(peerWithFlag.Name))
		if err != nil {
			return xerrors.Errorf("parsing peer-with: %w", err)
		}

		ds, err := bds.NewDatastore("./datastore", nil)
		if err != nil {
			return xerrors.Errorf("opening datastore: %w", err)
		}

		priv, err := lp2p.LoadOrCreatePrivKey(cctx.String(idFlag.Name))
		if err != nil {
			return xerrors.Errorf("loading p2p key: %w", err)
		}

		h, ps, err := lp2p.ConstructHost(ds, priv, cctx.String("listen"), bootstrap)
		if err != nil {
			return xerrors.Errorf("constructing host: %w", err)
		}

		addrs, err := h.Network().InterfaceListenAddresses()
		if err != nil {
			return xerrors.Errorf("getting InterfaceListenAddresses: %w", err)
		}

		for _, a := range addrs {
			fmt.Printf("%s/p2p/%s\n", a, h.ID())
		}

		t, err := ps.Join(lp2p.PubSubTopic(cctx.String("network-name")))
		if err != nil {
			return xerrors.Errorf("joining topic: %w", err)
		}

		opts := []grpc.DialOption{}
		if cctx.IsSet("cert") {
			creds, err := credentials.NewClientTLSFromFile(cctx.String("cert"), "")
			if err != nil {
				return xerrors.Errorf("loading cert file: %w", err)
			}
			opts = append(opts, grpc.WithTransportCredentials(creds))
		} else if cctx.Bool("insecure") {
			opts = append(opts, grpc.WithInsecure())
		} else {
			opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
		}

		for {
			conn, err := grpc.Dial(cctx.String("connect"), opts...)
			if err != nil {
				log.Warnf("error connecting to grpc: %+v", err)
				time.Sleep(5 * time.Second)
				continue
			}
			client := drand.NewPublicClient(conn)
			err = workRelay(client, t)
			if err != nil {
				log.Warnf("error relaying: %+v", err)
				time.Sleep(5 * time.Second)
			}
		}

		return nil
	},
}

func workRelay(client drand.PublicClient, t *pubsub.Topic) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	curr, err := client.PublicRand(ctx, &drand.PublicRandRequest{Round: 0})
	if err != nil {
		return xerrors.Errorf("getting initial round failed: %w", err)
	}
	log.Infof("got latest rand: %d", curr.Round)

	// context.Background() on purpose as this applies to whole, long lived stream
	stream, err := client.PublicRandStream(context.Background(), &drand.PublicRandRequest{Round: curr.Round})
	if err != nil {
		return xerrors.Errorf("getting rand stream: %w", err)
	}

	for {
		rand, err := stream.Recv()
		if err != nil {
			return xerrors.Errorf("receving on stream: %w", err)
		}

		randB, err := proto.Marshal(rand)
		if err != nil {
			return xerrors.Errorf("marshaling: %w", err)
		}

		err = t.Publish(context.TODO(), randB)
		if err != nil {
			return xerrors.Errorf("publishing on pubsub: %w", err)
		}
		log.Infof("Published randomness on pubsub, round: %d", rand.Round)
	}

}

var clientCmd = &cli.Command{
	Name:  "client",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		bootstrap, err := parseMultiaddrSlice(cctx.StringSlice(peerWithFlag.Name))
		if err != nil {
			return xerrors.Errorf("parsing peer-with: %w", err)
		}

		priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return xerrors.Errorf("generating ed25519 key: %w", err)
		}

		_, ps, err := lp2p.ConstructHost(datastore.NewMapDatastore(), priv, "/ip4/0.0.0.0/tcp/0", bootstrap)
		if err != nil {
			return xerrors.Errorf("constructing host: %w", err)
		}

		c, err := client.NewWithPubsub(ps, cctx.String("network-name"))
		if err != nil {
			return xerrors.Errorf("constructing client: %w", err)
		}

		var notifChan <-chan drand.PublicRandResponse
		var unsub client.UnsubFunc
		{
			ch := make(chan drand.PublicRandResponse, 5)
			notifChan = ch
			unsub = c.Sub(ch)
		}
		_ = unsub

		for rand := range notifChan {
			fmt.Printf("got randomness: Round %d: %X\n", rand.Round, rand.Signature[:16])
		}
		return nil
	},
}

var idFlag = &cli.StringFlag{
	Name:  "identity",
	Usage: "path to a file containing libp2p identity",
	Value: "identity.key",
}

var idCmd = &cli.Command{
	Name:  "peerid",
	Usage: "prints libp2p peerid",

	Flags: []cli.Flag{idFlag},
	Action: func(cctx *cli.Context) error {
		priv, err := lp2p.LoadOrCreatePrivKey(cctx.String(idFlag.Name))
		if err != nil {
			return xerrors.Errorf("loading p2p key: %w", err)
		}
		peerId, err := peer.IDFromPrivateKey(priv)
		if err != nil {
			return xerrors.Errorf("computing peerid: %w", err)
		}
		fmt.Printf("%s\n", peerId)
		return nil
	},
}
