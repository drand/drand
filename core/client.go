package core

import (
	"github.com/dedis/drand/beacon"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	"github.com/dedis/drand/protobuf/drand"
	"github.com/dedis/kyber/sign/bls"
)

type Client struct {
	conf   *Config
	client net.Client
	public *key.DistPublic
	p      net.Peer
}

func NewClient(conf *Config, public *key.DistPublic, server string) *Client {
	return &Client{
		conf:   conf,
		client: net.NewGrpcClient(conf.grpcOpts...),
		p:      &peerAddr{server},
		public: public,
	}
}

// Last returns the last randomness beacon from the server associated. It
// returns it if the randomness is valid.
func (c *Client) Last() (*drand.PublicRandResponse, error) {
	resp, err := c.client.Public(c.p, &drand.PublicRandRequest{})
	if err != nil {
		return nil, err
	}
	return resp, c.verify(resp)
}

func (c *Client) verify(resp *drand.PublicRandResponse) error {
	msg := beacon.Message(resp.PreviousSig, resp.Timestamp)
	return bls.Verify(key.Pairing, c.public.Key, msg, resp.Signature)
}

type peerAddr struct {
	addr string
}

func (p *peerAddr) Address() string {
	return p.addr
}
