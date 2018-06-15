package api

import (
	"github.com/dedis/drand/beacon"
	"github.com/dedis/drand/ecies"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	"github.com/dedis/drand/protobuf/drand"
	"github.com/dedis/kyber"
	"github.com/dedis/kyber/sign/bls"
	"google.golang.org/grpc"
)

// Client is the endpoint logic, communicating with drand servers
type Client struct {
	client net.ExternalClient
	public *key.DistPublic
}

// NewGrpcClient returns a Client able to talk to drand instances using gRPC
// communication method
func NewGrpcClient(opts ...grpc.DialOption) *Client {
	return &Client{
		client: net.NewGrpcClient(opts...),
	}
}

func NewRESTClient() *Client {
	return &Client{
		client: net.NewRestClient(),
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
	obj, err := ecies.Encrypt(key.G2, ecies.DefaultHash, id.Key, ephBuff)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Private(&peerAddr{id.Addr}, &drand.PrivateRandRequest{obj})
	if err != nil {
		return nil, err
	}
	return ecies.Decrypt(key.G2, ecies.DefaultHash, ephScalar, resp.GetResponse())
}

func (c *Client) verify(public kyber.Point, resp *drand.PublicRandResponse) error {
	msg := beacon.Message(resp.GetPrevious(), resp.GetRound())
	return bls.Verify(key.Pairing, public, msg, resp.GetRandomness())
}

type peerAddr struct {
	addr string
}

func (p *peerAddr) Address() string {
	return p.addr
}
