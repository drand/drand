package net

// Peer is a simple interface that allows retrieving the address of a
// destination. It might further be enhanced with certificates properties and
// all.
type Peer interface {
	Address() string
	IsTLS() bool
}

type sPeer struct {
	addr string
	tls  bool
}

func (s *sPeer) Address() string {
	return s.addr
}

func (s *sPeer) IsTLS() bool {
	return s.tls
}

func CreatePeer(addr string, tls bool) Peer {
	return &sPeer{
		addr: addr,
		tls:  tls,
	}
}
