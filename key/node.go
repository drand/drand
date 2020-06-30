package key

import (
	"encoding/binary"

	dkg "github.com/drand/kyber/share/dkg"

	proto "github.com/drand/drand/protobuf/drand"
)

// Index is the index of the node
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

// Hash is a compact representation of the node
func (n *Node) Hash() []byte {
	h := hashFunc()
	_ = binary.Write(h, binary.LittleEndian, n.Index)
	_, _ = n.Identity.Key.MarshalTo(h)
	return h.Sum(nil)
}

// Equal indicates if two nodes are equal
func (n *Node) Equal(n2 *Node) bool {
	return n.Index == n2.Index && n.Identity.Equal(n2.Identity)
}

// TOML is a toml representation of the node
func (n *Node) TOML() interface{} {
	return &NodeTOML{
		PublicTOML: n.Identity.TOML().(*PublicTOML),
		Index:      n.Index,
	}
}

// FromTOML unmarshals a node from TOML representation
func (n *Node) FromTOML(t interface{}) error {
	ntoml := t.(*NodeTOML)
	n.Index = ntoml.Index
	n.Identity = new(Identity)
	return n.Identity.FromTOML(ntoml.PublicTOML)
}

// TOMLValue is used in marshaling
func (n *Node) TOMLValue() interface{} {
	return new(NodeTOML)
}

// NodeTOML is the node's toml representation
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
