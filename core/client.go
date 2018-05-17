package core

import (
	"github.com/dedis/drand/core/beacon"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	"github.com/dedis/drand/protobuf/drand"
	"github.com/dedis/kyber"
	"github.com/dedis/kyber/sign/bls"
	"google.golang.org/grpc"
)

// Client is the endpoint logic, communicating with drand servers
type Client struct {
	client net.Client
	public *key.DistPublic
}

// NewClient returns a Client able to talk to drand instances
func NewClient(opts ...grpc.DialOption) *Client {
	return &Client{
		client: net.NewGrpcClient(opts...),
	}
}

// LastPublic returns the last randomness beacon from the server associated. It
// returns it if the randomness is valid.
func (c *Client) LastPublic(addr string, pub *key.DistPublic) (*drand.PublicRandResponse, error) {
	resp, err := c.client.Public(&peerAddr{addr}, &drand.PublicRandRequest{})
	if err != nil {
		return nil, err
	}
	return resp, c.verify(pub.Key, resp)
}

// Private retrieves a private random value from the server. It does that by
// generating an ephemeral key pair, sends it encrypted to the remote server,
// and decrypts the response, the randomness.
func (c *Client) Private(id *key.Identity) ([]byte, error) {
	ephScalar := key.G2.Scalar()
	ephPoint := key.G2.Point().Mul(ephScalar, nil)
	ephBuff, err := ephPoint.MarshalBinary()
	if err != nil {
		return nil, err
	}
	obj, err := Encrypt(key.G2, DefaultHash, id.Key, ephBuff)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Private(&peerAddr{id.Addr}, &drand.PrivateRandRequest{obj})
	if err != nil {
		return nil, err
	}
	return Decrypt(key.G2, DefaultHash, ephScalar, resp.GetResponse())
}

func (c *Client) verify(public kyber.Point, resp *drand.PublicRandResponse) error {
	msg := beacon.Message(resp.GetPreviousRand(), resp.GetRound())
	return bls.Verify(key.Pairing, public, msg, resp.GetRandomness())
}

type peerAddr struct {
	addr string
}

func (p *peerAddr) Address() string {
	return p.addr
}
