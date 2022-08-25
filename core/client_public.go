package core

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/encrypt/ecies"
)

// Client is the endpoint logic, communicating with drand servers
// XXX: This API should go away. Do not extend any further.
type Client struct {
	client    net.PublicClient
	chainHash []byte
}

// NewGrpcClient returns a Client able to talk to drand instances using gRPC
// communication method
func NewGrpcClient(chainHash []byte, opts ...grpc.DialOption) *Client {
	return &Client{
		client:    net.NewGrpcClient(opts...),
		chainHash: chainHash,
	}
}

// NewGrpcClientFromCert returns a client that contact its peer over TLS
func NewGrpcClientFromCert(chainHash []byte, c *net.CertManager, opts ...grpc.DialOption) *Client {
	return &Client{
		client:    net.NewGrpcClientFromCertManager(c, opts...),
		chainHash: chainHash,
	}
}

// ChainInfo returns the chain info as reported by the given peer.
func (c *Client) ChainInfo(p net.Peer) (*chain.Info, error) {
	metadata := common.Metadata{ChainHash: c.chainHash}
	resp, err := c.client.ChainInfo(context.TODO(), p, &drand.ChainInfoRequest{Metadata: &metadata})
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
		return nil, fmt.Errorf("failed to marshal ephemeral point: %w", err)
	}
	obj, err := ecies.Encrypt(key.KeyGroup, id.Key, ephBuff, EciesHash)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt ephemeral key: %w", err)
	}
	resp, err := c.client.PrivateRand(context.TODO(), id,
		&drand.PrivateRandRequest{Request: obj, Metadata: &common.Metadata{ChainHash: c.chainHash}})
	if err != nil {
		return nil, fmt.Errorf("failed to get private rand: %w", err)
	}
	return ecies.Decrypt(key.KeyGroup, ephScalar, resp.GetResponse(), EciesHash)
}
