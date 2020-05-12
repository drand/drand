package key

import (
	"encoding/binary"

	dkg "github.com/drand/kyber/share/dkg"

	proto "github.com/drand/drand/protobuf/drand"
)

type Index = dkg.Index

// Node is a wrapper around identity that additionally includes the index that
// the node has within this group. The index is computed initially when the
// group is first created. The index is useful only for drand nodes, and
// shouldn't be taken into account from an external point of view.
// The index is useful to be able to reshare correctly, and gives the ability to
// have a better logging: packets sent during DKG only contain an index, from
// which we can derive the actual address from the index.
type Node struct {
	*Identity
	Index Index
}

func (n *Node) Hash() []byte {
	h := hashFunc()
	binary.Write(h, binary.LittleEndian, n.Index)
	buff, _ := n.Identity.Key.MarshalBinary()
	h.Write(buff)
	return h.Sum(nil)
}

func (n *Node) Equal(n2 *Node) bool {
	return n.Index == n2.Index && n.Identity.Equal(n2.Identity)
}

func (n *Node) TOML() interface{} {
	return &NodeTOML{
		PublicTOML: n.Identity.TOML().(*PublicTOML),
		Index:      n.Index,
	}
}

func (n *Node) FromTOML(t interface{}) error {
	ntoml := t.(*NodeTOML)
	n.Index = ntoml.Index
	n.Identity = new(Identity)
	return n.Identity.FromTOML(ntoml.PublicTOML)
}

func (n *Node) TOMLValue() interface{} {
	return new(NodeTOML)
}

type NodeTOML struct {
	*PublicTOML
	Index Index
}

// NodeFromProto creates a node from its wire representation
func NodeFromProto(n *proto.Node) (*Node, error) {
	id, err := IdentityFromProto(n.Public)
	if err != nil {
		return nil, err
	}
	return &Node{
		Index:    n.Index,
		Identity: id,
	}, nil
}
