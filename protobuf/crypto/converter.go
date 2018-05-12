// converter implements some routines to go back and forth from a protobuf
// point and scalar to a kyber Point and Scalar interface.
package crypto

import (
	"errors"
	fmt "fmt"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/suites"
)

type ProtobufPoint = Point
type ProtobufScalar = Scalar

func ProtoToKyberPoint(p *ProtobufPoint) (kyber.Point, error) {
	groupName, exists := GroupID_name[int32(p.GetGid())]
	if !exists {
		return nil, fmt.Errorf("oid %d unknown", p.GetGid())
	}
	group, err := suites.Find(groupName)
	if err != nil {
		return nil, fmt.Errorf("group name %s unknown", groupName)
	}
	point := group.Point()
	return point, point.UnmarshalBinary(p.GetData())
}

func KyberToProtoPoint(p kyber.Point) (*ProtobufPoint, error) {
	desc, ok := p.(kyber.Groupable)
	if !ok {
		return nil, errors.New("given point is not self describing")
	}
	group := desc.Group()
	gid, exists := GroupID_value[group.String()]
	if !exists {
		return nil, fmt.Errorf("group %s is not registered to the protobuf mapping", group.String())
	}
	buffer, err := p.MarshalBinary()
	return &ProtobufPoint{
		Gid:  GroupID(gid),
		Data: buffer,
	}, err
}

func ProtoToKyberScalar(p *ProtobufScalar) (kyber.Scalar, error) {
	groupName, exists := GroupID_name[int32(p.GetGid())]
	if !exists {
		return nil, fmt.Errorf("oid %d unknown")
	}
	group, err := suites.Find(groupName)
	if err != nil {
		return nil, fmt.Errorf("group name %s unknown", groupName)
	}
	scalar := group.Scalar()
	return scalar, scalar.UnmarshalBinary(p.GetData())
}

func KyberToProtoScalar(s kyber.Scalar) (*ProtobufScalar, error) {
	desc, ok := s.(kyber.Groupable)
	if !ok {
		return nil, errors.New("given point is not self describing")
	}
	group := desc.Group()
	gid, exists := GroupID_value[group.String()]
	if !exists {
		return nil, fmt.Errorf("group %s is not registered to the protobuf mapping", group.String())
	}
	buffer, err := s.MarshalBinary()
	return &ProtobufScalar{
		Gid:  GroupID(gid),
		Data: buffer,
	}, err
}
