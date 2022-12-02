package util

import (
	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share/dkg"
	"reflect"
	"sort"
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

func EqualParticipant(p1 *drand.Participant, p2 *drand.Participant) bool {
	return p1.Tls == p2.Tls && p1.Address == p2.Address && reflect.DeepEqual(p1.PubKey, p2.PubKey) && reflect.DeepEqual(p1.Signature, p2.Signature)
}

func PublicKeyAsParticipant(identity *key.Identity) (*drand.Participant, error) {
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

func ToNode(index int, participant *drand.Participant) (dkg.Node, error) {
	public, err := pkToPoint(participant.PubKey)
	if err != nil {
		return dkg.Node{}, err
	}
	return dkg.Node{
		Public: public,
		Index:  uint32(index),
	}, nil
}

func ToKeyNode(index int, participant *drand.Participant) (key.Node, error) {
	public, err := pkToPoint(participant.PubKey)
	if err != nil {
		return key.Node{}, err
	}

	return key.Node{
		Identity: &key.Identity{
			Key:       public,
			Addr:      participant.Address,
			TLS:       participant.Tls,
			Signature: participant.Signature,
		},
		Index: uint32(index),
	}, nil
}

func pkToPoint(pk []byte) (kyber.Point, error) {
	point := key.KeyGroup.Point()
	if err := point.UnmarshalBinary(pk); err != nil {
		return nil, err
	}
	return point, nil
}

func SortedByPublicKey(arr []*drand.Participant) []*drand.Participant {
	out := arr
	sort.Slice(out, func(i, j int) bool {
		return string(arr[i].PubKey) < string(arr[j].PubKey)
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
