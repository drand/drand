package util

import (
	"bytes"
	"sort"

	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/net"
	drand "github.com/drand/drand/v2/protobuf/dkg"
	proto "github.com/drand/drand/v2/protobuf/drand"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share/dkg"
)

func Contains(haystack []*drand.Participant, needle *drand.Participant) bool {
	if haystack == nil {
		return false
	}
	for _, v := range haystack {
		if EqualParticipant(v, needle) {
			return true
		}
	}
	return false
}

func ContainsAll(haystack, needles []*drand.Participant) bool {
	found := make(map[string]bool)

	for _, participant := range haystack {
		found[participant.Address] = true
	}
	for _, needle := range needles {
		if !found[needle.Address] {
			return false
		}
	}

	return true
}

// Without removes needle from the haystack. Careful: it modifies the input slice but also returns the resulting slice.
// It removes all instances of needle, and zeros the removed items to allow garbage collection.
func Without(haystack []*drand.Participant, needle *drand.Participant) []*drand.Participant {
	if haystack == nil {
		return nil
	}

	// ret will reuse the underlying array of haystack
	ret := haystack[:0]
	for _, v := range haystack {
		if EqualParticipant(v, needle) {
			continue
		}
		ret = append(ret, v)
	}
	// we let the deleted items get garbage collected
	for i := len(ret); i < len(haystack); i++ {
		haystack[i] = nil
	}

	if len(ret) == 0 {
		return nil
	}

	return ret
}

func EqualParticipant(p1, p2 *drand.Participant) bool {
	// we use the Getters sibce tgey handle the nil cases
	return p1.GetAddress() == p2.GetAddress() &&
		bytes.Equal(p1.GetKey(), p2.GetKey()) &&
		bytes.Equal(p1.GetSignature(), p2.GetSignature())
}

func PublicKeyAsParticipant(identity *key.Identity) (*drand.Participant, error) {
	pubKey, err := identity.Key.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return &drand.Participant{
		Address:   identity.Address(),
		Key:       pubKey,
		Signature: identity.Signature,
	}, nil
}

func ToNode(index int, participant *drand.Participant, sch *crypto.Scheme) (dkg.Node, error) {
	// if this conversion fails, it's almost certain the nodes are using mismatched schemes
	public, err := pkToPoint(participant.Key, sch)
	if err != nil {
		return dkg.Node{}, key.ErrInvalidKeyScheme
	}
	return dkg.Node{
		Public: public,
		Index:  uint32(index),
	}, nil
}

func ToParticipant(node *proto.Node) *drand.Participant {
	return &drand.Participant{
		Address:   node.Public.Address,
		Key:       node.Public.Key,
		Signature: node.Public.Signature,
	}
}

func ToKeyNode(index int, participant *drand.Participant, sch *crypto.Scheme) (key.Node, error) {
	// if this conversion fails, it's almost certain the nodes are using mismatched schemes
	public, err := pkToPoint(participant.Key, sch)
	if err != nil {
		return key.Node{}, key.ErrInvalidKeyScheme
	}

	return key.Node{
		Identity: &key.Identity{
			Key:       public,
			Addr:      participant.Address,
			Signature: participant.Signature,
			Scheme:    sch,
		},
		Index: uint32(index),
	}, nil
}

func ToPeer(participant *drand.Participant) net.Peer {
	return net.CreatePeer(participant.Address)
}

func pkToPoint(pk []byte, sch *crypto.Scheme) (kyber.Point, error) {
	point := sch.KeyGroup.Point()
	if err := point.UnmarshalBinary(pk); err != nil {
		return nil, err
	}
	return point, nil
}

func SortedByPublicKey(arr []*drand.Participant) []*drand.Participant {
	out := arr
	sort.Slice(out, func(i, j int) bool {
		return string(out[i].Key) < string(out[j].Key)
	})
	return out
}

func TryMapEach[T any](arr []*drand.Participant, fn func(index int, participant *drand.Participant) (T, error)) ([]T, error) {
	out := make([]T, len(arr))
	for i, participant := range arr {
		p := participant
		result, err := fn(i, p)
		if err != nil {
			return nil, err
		}
		out[i] = result
	}
	return out, nil
}

func NonEmpty(p *drand.Participant) bool {
	return p != nil && p.Address != ""
}
