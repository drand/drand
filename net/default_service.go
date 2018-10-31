package net

import (
	"context"

	"github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/protobuf/drand"
)

// Default service implements the Service interface with methods that returns empty messages.
// Default service is useful mainly for testing, where you want to implement only a specific functionality of a Service.
// To use : depending on which server you want to test, define a struct that implemants BeaconServer, RandomnessServer or DkgServer
// and instanciate defaultService with &DefaultService{<your struct>}.
type DefaultService struct {
	B drand.BeaconServer
	R drand.RandomnessServer
	I drand.InfoServer
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
func (s *DefaultService) DistKey(c context.Context, in *drand.DistKeyRequest) (*drand.DistKeyResponse, error) {
	if s.I == nil {
		return &drand.DistKeyResponse{}, nil
	} else {
		return s.I.DistKey(c, in)
	}
}
func (s *DefaultService) Setup(c context.Context, in *dkg.DKGPacket) (*dkg.DKGResponse, error) {
	if s.D != nil {
		return s.D.Setup(c, in)
	}
	return &dkg.DKGResponse{}, nil
}

func (s *DefaultService) Reshare(c context.Context, in *dkg.ResharePacket) (*dkg.ReshareResponse, error) {
	if s.D != nil {
		return s.D.Reshare(c, in)
	}
	return &dkg.ReshareResponse{}, nil
}
func (s *DefaultService) NewBeacon(c context.Context, in *drand.BeaconRequest) (*drand.BeaconResponse, error) {
	if s.B == nil {
		return &drand.BeaconResponse{}, nil
	} else {
		return s.B.NewBeacon(c, in)
	}
}
