// Package crypto implements some routines to go back and forth from a protobuf
// point and scalar to a kyber Point and Scalar interface, as well as standard
// JSON representations.
package crypto

import (
	"errors"
	fmt "fmt"

	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/suites"
)

// ProtobufPoint is an alias to a Point represented in a protobuf packet
type ProtobufPoint = Point

// ProtobufScalar is an alias to a Scalar represented in a protobuf packet
type ProtobufScalar = Scalar

// ToKyber returns the kyber represnetation of this protobuf scalar
func (p *ProtobufScalar) ToKyber() (kyber.Scalar, error) {
	return ProtoToKyberScalar(p)
}

// FromKyber unmarshals the protobuf scalar from the kyber representation
func (p *ProtobufScalar) FromKyber(s kyber.Scalar) error {
	p2, err := KyberToProtoScalar(s)
	*p = *p2
	return err
}

// ToKyber returns the kyber represnetation of this protobuf point
func (p *ProtobufPoint) ToKyber() (kyber.Point, error) {
	return ProtoToKyberPoint(p)
}

// FromKyber unmarshals the protobuf point from the kyber representation
func (p *ProtobufPoint) FromKyber(point kyber.Point) error {
	p2, err := KyberToProtoPoint(point)
	*p = *p2
	return err
}

// ProtoToKyberPoint converts a protobuf point to a kyber point
func ProtoToKyberPoint(p *ProtobufPoint) (kyber.Point, error) {
	group, exists := IDToGroup(int32(p.GetGid()))
	if !exists {
		return nil, fmt.Errorf("oid %d unknown", p.GetGid())
	}
	point := group.Point()
	return point, point.UnmarshalBinary(p.GetPoint())
}

// KyberToProtoPoint converts a kyber point to a protobuf scalar
func KyberToProtoPoint(p kyber.Point) (*ProtobufPoint, error) {
	desc, ok := p.(kyber.Groupable)
	if !ok {
		return nil, errors.New("given point is not self describing")
	}
	group := desc.Group()
	gid, exists := GroupToID(group)
	if !exists {
		return nil, fmt.Errorf("group %s is not registered to the protobuf mapping", group.String())
	}
	buffer, err := p.MarshalBinary()
	return &ProtobufPoint{
		Gid:   GroupID(gid),
		Point: buffer,
	}, err
}

// ProtoToKyberScalar converts a protobuf scalar to a kyber scalar
func ProtoToKyberScalar(p *ProtobufScalar) (kyber.Scalar, error) {
	group, exists := IDToGroup(int32(p.GetGid()))
	if !exists {
		return nil, fmt.Errorf("group %d unknown", p.GetGid())
	}
	scalar := group.Scalar()
	return scalar, scalar.UnmarshalBinary(p.GetScalar())
}

// KyberToProtoScalar converts a kyber scalar to a protobuf scalar
func KyberToProtoScalar(s kyber.Scalar) (*ProtobufScalar, error) {
	desc, ok := s.(kyber.Groupable)
	if !ok {
		return nil, errors.New("given point is not self describing")
	}
	group := desc.Group()
	gid, exists := GroupToID(group)
	if !exists {
		return nil, fmt.Errorf("group %s is not registered to the protobuf mapping", group.String())
	}
	buffer, err := s.MarshalBinary()
	return &ProtobufScalar{
		Gid:    GroupID(gid),
		Scalar: buffer,
	}, err
}

// GroupToID returns the ID of a group
func GroupToID(g kyber.Group) (int32, bool) {
	gid, exists := GroupID_value[g.String()]
	return gid, exists
}

// IDToGroup returns the kyber.Group corresponding to the given ID if provided
func IDToGroup(id int32) (kyber.Group, bool) {
	groupName, exists := GroupID_name[id]
	if !exists {
		return nil, false
	}
	group, err := suites.Find(groupName)
	if err != nil {
		return nil, false
	}
	return group, true
}
