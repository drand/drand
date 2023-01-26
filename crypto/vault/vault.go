package vault

import (
	"sync"

	"github.com/drand/drand/crypto"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/kyber/share"
)

// CryptoSafe holds the cryptographic information to generate a partial beacon
type CryptoSafe interface {
	// SignPartial returns the partial signature
	SignPartial(msg []byte) ([]byte, error)
}

// Vault stores the information necessary to validate partial beacon, full
// beacons and to sign new partial beacons (it implements CryptoSafe interface).
// Vault is thread safe when using the methods.
type Vault struct {
	mu sync.RWMutex
	*crypto.Scheme
	// current share of the node
	share *key.Share
	// public polynomial to verify a partial beacon
	pub *share.PubPoly
	// chian info to verify final random beacon
	chain *chain.Info
	// to know the threshold, transition time etc
	group *key.Group
}

func NewVault(currentGroup *key.Group, ks *key.Share, sch *crypto.Scheme) *Vault {
	return &Vault{
		Scheme: sch,
		chain:  chain.NewChainInfo(currentGroup),
		share:  ks,
		pub:    currentGroup.PublicKey.PubPoly(sch),
		group:  currentGroup,
	}
}

// GetGroup returns the current group
func (v *Vault) GetGroup() *key.Group {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.group
}

func (v *Vault) GetPub() *share.PubPoly {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.pub
}

func (v *Vault) GetInfo() *chain.Info {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.chain
}

// SignPartial implemements the CryptoSafe interface
func (v *Vault) SignPartial(msg []byte) ([]byte, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.Scheme.ThresholdScheme.Sign(v.share.PrivateShare(), msg)
}

// Index returns the index of the share
func (v *Vault) Index() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.share.Share.I
}

func (v *Vault) SetInfo(newGroup *key.Group, ks *key.Share) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.share = ks
	v.group = newGroup
	v.pub = newGroup.PublicKey.PubPoly(v.Scheme)
	// v.chain info is constant
	// Scheme cannot change either
}
