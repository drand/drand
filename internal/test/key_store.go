package test

import (
	"github.com/drand/drand/common/crypto"
	key2 "github.com/drand/drand/common/key"
)

type KeyStore struct {
	priv  *key2.Pair
	share *key2.Share
	group *key2.Group
	dist  *key2.DistPublic
}

func NewKeyStore() key2.Store {
	return &KeyStore{}
}

func (k *KeyStore) SaveKeyPair(p *key2.Pair) error {
	k.priv = p
	return nil
}

func (k *KeyStore) LoadKeyPair(_ *crypto.Scheme) (*key2.Pair, error) {
	return k.priv, nil
}

func (k *KeyStore) SaveShare(share *key2.Share) error {
	k.share = share
	return nil
}

func (k *KeyStore) LoadShare(_ *crypto.Scheme) (*key2.Share, error) {
	return k.share, nil
}

func (k *KeyStore) SaveGroup(g *key2.Group) error {
	k.group = g
	return nil
}

func (k *KeyStore) LoadGroup() (*key2.Group, error) {
	return k.group, nil
}

func (k *KeyStore) SaveDistPublic(d *key2.DistPublic) error {
	k.dist = d
	return nil
}
func (k *KeyStore) LoadDistPublic() (*key2.DistPublic, error) {
	return k.dist, nil
}

func (k *KeyStore) Reset(_ ...key2.ResetOption) error {
	k.group = nil
	k.dist = nil
	k.share = nil
	return nil
}
