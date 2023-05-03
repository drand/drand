package util

import (
	"github.com/drand/drand/crypto"
	"reflect"
	"sort"

	key2 "github.com/drand/drand/common/key"
	"github.com/drand/drand/internal/net"
	"github.com/drand/drand/protobuf/drand"
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

func Without(haystack []*drand.Participant, needle *drand.Participant) []*drand.Participant {
	if haystack == nil {
		return nil
	}

	indexToRemove := -1
	for i, v := range haystack {
		if EqualParticipant(v, needle) {
			indexToRemove = i
		}
	}

	if indexToRemove == -1 {
		return haystack
	}

	if len(haystack) == 1 {
		return nil
	}

	var ret []*drand.Participant
	ret = append(ret, haystack[:indexToRemove]...)
	return append(ret, haystack[indexToRemove+1:]...)
}

func EqualParticipant(p1, p2 *drand.Participant) bool {
	if p1 == nil || p2 == nil {
		return false
	}
	return p1.Tls == p2.Tls &&
		p1.Address == p2.Address &&
		reflect.DeepEqual(p1.PubKey, p2.PubKey) &&
		reflect.DeepEqual(p1.Signature, p2.Signature)
}

func PublicKeyAsParticipant(identity *key2.Identity) (*drand.Participant, error) {
	pubKey, err := identity.Key.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return &drand.Participant{
		Address:   identity.Address(),
		Tls:       identity.TLS,
		PubKey:    pubKey,
		Signature: identity.Signature,
	}, nil
}

func ToNode(index int, participant *drand.Participant, sch *crypto.Scheme) (dkg.Node, error) {
	// if this conversion fails, it's almost certain the nodes are using mismatched schemes
	public, err := pkToPoint(participant.PubKey, sch)
	if err != nil {
		return dkg.Node{}, key2.ErrInvalidKeyScheme
	}
	return dkg.Node{
		Public: public,
		Index:  uint32(index),
	}, nil
}

func ToKeyNode(index int, participant *drand.Participant, sch *crypto.Scheme) (key2.Node, error) {
	// if this conversion fails, it's almost certain the nodes are using mismatched schemes
	public, err := pkToPoint(participant.PubKey, sch)
	if err != nil {
		return key2.Node{}, key2.ErrInvalidKeyScheme
	}

	return key2.Node{
		Identity: &key2.Identity{
			Key:       public,
			Addr:      participant.Address,
			TLS:       participant.Tls,
			Signature: participant.Signature,
			Scheme:    sch,
		},
		Index: uint32(index),
	}, nil
}

func ToPeer(participant *drand.Participant) net.Peer {
	return net.CreatePeer(participant.Address, participant.Tls)
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
		return string(out[i].PubKey) < string(out[j].PubKey)
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
