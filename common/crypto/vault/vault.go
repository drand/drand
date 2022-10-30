package vault

import (
	"sync"

	"github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/crypto"
	key2 "github.com/drand/drand/common/key"
	"github.com/drand/drand/common/log"
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
	log log.Logger
	mu  sync.RWMutex
	*crypto.Scheme
	// current share of the node
	share *key2.Share
	// public polynomial to verify a partial beacon
	pub *share.PubPoly
	// chain info to verify final random beacon
	chain *chain.Info
	// to know the threshold, transition time etc
	group *key2.Group
}

func NewVault(l log.Logger, currentGroup *key2.Group, ks *key2.Share, sch *crypto.Scheme) *Vault {
	return &Vault{
		log:    l,
		Scheme: sch,
		chain:  chain.NewChainInfo(l, currentGroup),
		share:  ks,
		pub:    currentGroup.PublicKey.PubPoly(sch),
		group:  currentGroup,
	}
}

// GetGroup returns the current group
func (v *Vault) GetGroup() *key2.Group {
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

func (v *Vault) SetInfo(newGroup *key2.Group, ks *key2.Share) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.share = ks
	v.group = newGroup
	v.pub = newGroup.PublicKey.PubPoly(v.Scheme)
	// v.chain info is constant
	// Scheme cannot change either
}
