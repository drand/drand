package core

import (
	"errors"

	"github.com/dedis/drand/beacon"
	"github.com/dedis/drand/ecies"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	"github.com/dedis/drand/protobuf/crypto"
	"github.com/dedis/drand/protobuf/drand"
	"go.dedis.ch/kyber/sign/bls"
	"go.dedis.ch/kyber/v3"
	"google.golang.org/grpc"
)

// Client is the endpoint logic, communicating with drand servers
// XXX: This API should go away. Do not extend any further.
type Client struct {
	client net.ExternalClient
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
	resp, err := c.client.Public(&peerAddr{addr, secure}, &drand.PublicRandRequest{})
	if err != nil {
		return nil, err
	}
	return resp, c.verify(pub.Key(), resp)
}

// Public returns the random output of the specified beacon at a given index. It
// returns it if the randomness is valid. Secure indicates that the request
// must be made over a TLS protected channel.
func (c *Client) Public(addr string, pub *key.DistPublic, secure bool, round int) (*drand.PublicRandResponse, error) {
	resp, err := c.client.Public(&peerAddr{addr, secure}, &drand.PublicRandRequest{Round: uint64(round)})
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
	resp, err := c.client.Private(id, &drand.PrivateRandRequest{Request: obj})
	if err != nil {
		return nil, err
	}
	return ecies.Decrypt(key.G2, ecies.DefaultHash, ephScalar, resp.GetResponse())
}

// DistKey returns the distributed key the node at this address is holding.
func (c *Client) DistKey(addr string, secure bool) (*crypto.Point, error) {
	resp, err := c.client.DistKey(&peerAddr{addr, secure}, &drand.DistKeyRequest{})
	return resp.Key, err
}

// Group returns the group file used by the node in a JSON encoded format
func (c *Client) Group(addr string, secure bool) (*drand.GroupResponse, error) {
	return c.client.Group(&peerAddr{addr, secure}, &drand.GroupRequest{})
}

func (c *Client) verify(public kyber.Point, resp *drand.PublicRandResponse) error {
	msg := beacon.Message(resp.GetPrevious(), resp.GetRound())
	rand := resp.GetRandomness()
	if rand == nil {
		return errors.New("drand: no randomness found")
	}
	valid := bls.Verify(key.Pairing, public, msg, resp.GetSignature())
	/**hash := sha512.New()
	hash.Write(resp.GetSignature().GetPoint())
	randExpected := hash.Sum(nil)
	// TODO: HASH(SIG) == RAND
	//	valid := bls.Verify(key.Pairing, public, msg, resp.GetSig()) && randExpected == rand**/
	return valid
}

func (c *Client) peer(addr string) {

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
