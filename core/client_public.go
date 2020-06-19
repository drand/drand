package core

import (
	"context"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/encrypt/ecies"
	"google.golang.org/grpc"
)

// Client is the endpoint logic, communicating with drand servers
// XXX: This API should go away. Do not extend any further.
type Client struct {
	client net.PublicClient
}

// NewGrpcClient returns a Client able to talk to drand instances using gRPC
// communication method
func NewGrpcClient(opts ...grpc.DialOption) *Client {
	return &Client{
		client: net.NewGrpcClient(opts...),
	}
}

// NewGrpcClientFromCert returns a client that contact its peer over TLS
func NewGrpcClientFromCert(c *net.CertManager, opts ...grpc.DialOption) *Client {
	return &Client{client: net.NewGrpcClientFromCertManager(c, opts...)}
}

// ChainInfo returns the chain info as reported by the given peer.
func (c *Client) ChainInfo(p net.Peer) (*chain.Info, error) {
	resp, err := c.client.ChainInfo(context.TODO(), p, &drand.ChainInfoRequest{})
	if err != nil {
		return nil, err
	}
	return chain.InfoFromProto(resp)
}

// Private retrieves a private random value from the server. It does that by
// generating an ephemeral key pair, sends it encrypted to the remote server,
// and decrypts the response, the randomness. Client will attempt a TLS
// connection to the address in the identity if id.IsTLS() returns true
func (c *Client) Private(id *key.Identity) ([]byte, error) {
	ephScalar := key.KeyGroup.Scalar()
	ephPoint := key.KeyGroup.Point().Mul(ephScalar, nil)
	ephBuff, err := ephPoint.MarshalBinary()
	if err != nil {
		return nil, err
	}
	obj, err := ecies.Encrypt(key.KeyGroup, id.Key, ephBuff, EciesHash)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.PrivateRand(context.TODO(), id, &drand.PrivateRandRequest{Request: obj})
	if err != nil {
		return nil, err
	}
	return ecies.Decrypt(key.KeyGroup, ephScalar, resp.GetResponse(), EciesHash)
}
