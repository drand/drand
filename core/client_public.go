package core

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber"
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

// NewRESTClient returns a client that uses the HTTP Rest API delivered by drand
// nodes
func NewRESTClient() *Client {
	return &Client{
		client: net.NewRestClient(),
	}
}

// NewRESTClientFromCert returns a client that uses the HTTP Rest API delivered
// by drand nodes, using TLS connection for peers registered
func NewRESTClientFromCert(c *net.CertManager) *Client {
	return &Client{client: net.NewRestClientFromCertManager(c)}
}

// LastPublic returns the last randomness beacon from the server associated. It
// returns it if the randomness is valid. Secure indicates that the request
// must be made over a TLS protected channel.
func (c *Client) LastPublic(addr string, pub *key.DistPublic, secure bool) (*drand.PublicRandResponse, error) {
	resp, err := c.client.PublicRand(context.TODO(), &peerAddr{addr, secure}, &drand.PublicRandRequest{})
	if err != nil {
		return nil, err
	}
	return resp, c.verify(pub.Key(), resp)
}

// Public returns the random output of the specified beacon at a given index. It
// returns it if the randomness is valid. Secure indicates that the request
// must be made over a TLS protected channel.
func (c *Client) Public(addr string, pub *key.DistPublic, secure bool, round int) (*drand.PublicRandResponse, error) {
	resp, err := c.client.PublicRand(context.TODO(), &peerAddr{addr, secure}, &drand.PublicRandRequest{Round: uint64(round)})
	if err != nil {
		return nil, err
	}
	return resp, c.verify(pub.Key(), resp)
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

// DistKey returns the distributed key the node at this address is holding.
func (c *Client) DistKey(addr string, secure bool) (*drand.DistKeyResponse, error) {
	resp, err := c.client.DistKey(context.TODO(), &peerAddr{addr, secure}, &drand.DistKeyRequest{})
	return resp, err
}

// Group returns the group file used by the node in a JSON encoded format
func (c *Client) Group(addr string, secure bool) (*drand.GroupPacket, error) {
	return c.client.Group(context.TODO(), &peerAddr{addr, secure}, &drand.GroupRequest{})
}

func (c *Client) verify(public kyber.Point, resp *drand.PublicRandResponse) error {
	prevSig := resp.GetPreviousSignature()
	round := resp.GetRound()
	msg := beacon.Message(round, prevSig)
	rand := resp.GetRandomness()
	if rand == nil {
		return errors.New("drand: no randomness found")
	}
	ver := key.Scheme.VerifyRecovered(public, msg, resp.GetSignature())
	if ver != nil {
		return ver
	}
	expect := beacon.RandomnessFromSignature(resp.GetSignature())
	if !bytes.Equal(expect, rand) {
		exp := hex.EncodeToString(expect)[10:14]
		got := hex.EncodeToString(rand)[10:14]
		return fmt.Errorf("randomness: got %s , expected %s", got, exp)
	}
	return nil
}

type peerAddr struct {
	addr string
	t    bool
}

func (p *peerAddr) Address() string {
	return p.addr
}

func (p *peerAddr) IsTLS() bool {
	return p.t
}
