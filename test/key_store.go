package test

import "github.com/drand/drand/key"

type KeyStore struct {
	priv  *key.Pair
	share *key.Share
	group *key.Group
	dist  *key.DistPublic
}

func NewKeyStore() key.Store {
	return &KeyStore{}
}

func (k *KeyStore) SaveKeyPair(p *key.Pair) error {
	k.priv = p
	return nil
}

func (k *KeyStore) LoadKeyPair() (*key.Pair, error) {
	return k.priv, nil
}

func (k *KeyStore) SaveShare(share *key.Share) error {
	k.share = share
	return nil
}

func (k *KeyStore) LoadShare() (*key.Share, error) {
	return k.share, nil
}

func (k *KeyStore) SaveGroup(g *key.Group) error {
	k.group = g
	return nil
}

func (k *KeyStore) LoadGroup() (*key.Group, error) {
	return k.group, nil
}

func (k *KeyStore) SaveDistPublic(d *key.DistPublic) error {
	k.dist = d
	return nil
}
func (k *KeyStore) LoadDistPublic() (*key.DistPublic, error) {
	return k.dist, nil
}

func (k *KeyStore) Reset(...key.ResetOption) error {
	k.group = nil
	k.dist = nil
	k.share = nil
	return nil
}
