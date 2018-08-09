package net

import (
	"context"

	"github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/protobuf/drand"
)

type DefaultService struct {
	B drand.BeaconServer
	R drand.RandomnessServer
	D dkg.DkgServer
}

func (s *DefaultService) Public(c context.Context, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	if s.R == nil {
		return &drand.PublicRandResponse{}, nil
	} else {
		return s.R.Public(c, in)
	}
}
func (s *DefaultService) Private(c context.Context, in *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	if s.R == nil {
		return &drand.PrivateRandResponse{}, nil
	} else {
		return s.R.Private(c, in)
	}
}
func (s *DefaultService) Setup(c context.Context, in *dkg.DKGPacket) (*dkg.DKGResponse, error) {
	if s.D != nil {
		return s.D.Setup(c, in)
	}
	return &dkg.DKGResponse{}, nil
}
func (s *DefaultService) NewBeacon(c context.Context, in *drand.BeaconRequest) (*drand.BeaconResponse, error) {
	if s.B == nil {
		return &drand.BeaconResponse{}, nil
	} else {
		return s.B.NewBeacon(c, in)
	}
}
