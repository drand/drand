package beacon

import (
	"sync"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/kyber/share"
)

// CryptoSafe holds the cryptographic information to generate a partial beacon
type CryptoSafe interface {
	// SignPartial returns the partial signature
	SignPartial(msg []byte) ([]byte, error)
}

// cryptoStore stores the information necessary to validate partial beacon, full
// beacons and to sign new partial beacons (it implements CryptoSafe interface).
// cryptoStore is thread safe when using the methods.
type cryptoStore struct {
	sync.Mutex
	// current share of the node
	share *key.Share
	// public polynomial to verify a partial beacon
	pub *share.PubPoly
	// chian info to verify final random beacon
	chain *chain.Info
	// to know the threshold, transition time etc
	group *key.Group
}

func newCryptoStore(currentGroup *key.Group, ks *key.Share) *cryptoStore {
	return &cryptoStore{
		chain: chain.NewChainInfo(currentGroup),
		share: ks,
		pub:   currentGroup.PublicKey.PubPoly(),
		group: currentGroup,
	}
}

// GetGroup returns the current group
func (c *cryptoStore) GetGroup() *key.Group {
	c.Lock()
	defer c.Unlock()
	return c.group
}

func (c *cryptoStore) GetPub() *share.PubPoly {
	c.Lock()
	defer c.Unlock()
	return c.pub
}

// SignPartial implemements the CryptoSafe interface
func (c *cryptoStore) SignPartial(msg []byte) ([]byte, error) {
	c.Lock()
	defer c.Unlock()
	return key.Scheme.Sign(c.share.PrivateShare(), msg)
}

// Index returns the index of the share
func (c *cryptoStore) Index() int {
	return c.share.Share.I
}

func (c *cryptoStore) SetInfo(newGroup *key.Group, ks *key.Share) {
	c.Lock()
	defer c.Unlock()
	c.share = ks
	c.group = newGroup
	c.pub = newGroup.PublicKey.PubPoly()
	// chain info is constant
}
