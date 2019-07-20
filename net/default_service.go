package net

import (
	"context"

	"github.com/dedis/drand/protobuf/drand"
)

// DefaultService implements the Service interface with methods that returns
// empty messages.  Default service is useful mainly for testing, where you want
// to implement only a specific functionality of a Service.  To use : depending
// on which server you want to test, define a struct that implemants
// BeaconServer, RandomnessServer or DkgServer and instanciate defaultService
// with &DefaultService{<your struct>}.
type DefaultService struct {
	Pr drand.ProtocolServer
	Pu drand.PublicServer
	C  drand.ControlServer
}

// PublicRand ...
func (s *DefaultService) PublicRand(c context.Context, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	if s.Pu == nil {
		return &drand.PublicRandResponse{}, nil
	}
	return s.Pu.PublicRand(c, in)
}

// PrivateRand ...
func (s *DefaultService) PrivateRand(c context.Context, in *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	if s.Pu == nil {
		return &drand.PrivateRandResponse{}, nil
	}
	return s.Pu.PrivateRand(c, in)
}

// Group ...
func (s *DefaultService) Group(c context.Context, in *drand.GroupRequest) (*drand.GroupResponse, error) {
	if s.Pu == nil {
		return &drand.GroupResponse{}, nil
	}
	return s.Pu.Group(c, in)
}

// DistKey ...
func (s *DefaultService) DistKey(c context.Context, in *drand.DistKeyRequest) (*drand.DistKeyResponse, error) {
	if s.Pu == nil {
		return &drand.DistKeyResponse{}, nil
	}
	return s.Pu.DistKey(c, in)
}

// Setup ...
func (s *DefaultService) Setup(c context.Context, in *drand.SetupPacket) (*drand.Empty, error) {
	if s.Pr != nil {
		return s.Pr.Setup(c, in)
	}
	return new(drand.Empty), nil
}

// Reshare ...
func (s *DefaultService) Reshare(c context.Context, in *drand.ResharePacket) (*drand.Empty, error) {
	if s.Pr != nil {
		return s.Pr.Reshare(c, in)
	}
	return new(drand.Empty), nil
}

// NewBeacon ...
func (s *DefaultService) NewBeacon(c context.Context, in *drand.BeaconRequest) (*drand.BeaconResponse, error) {
	if s.Pr == nil {
		return &drand.BeaconResponse{}, nil
	}
	return s.Pr.NewBeacon(c, in)
}

// Home ...
func (s *DefaultService) Home(c context.Context, in *drand.HomeRequest) (*drand.HomeResponse, error) {
	if s.Pu == nil {
		return &drand.HomeResponse{}, nil
	}
	return s.Pu.Home(c, in)
}

// Control functionalities

// InitDKG ...
func (s *DefaultService) InitDKG(c context.Context, in *drand.DKGRequest) (*drand.DKGResponse, error) {
	if s.C == nil {
		return &drand.DKGResponse{}, nil
	}
	return s.C.InitDKG(c, in)
}

// InitReshare ...
func (s *DefaultService) InitReshare(c context.Context, in *drand.ReshareRequest) (*drand.ReshareResponse, error) {
	if s.C == nil {
		return &drand.ReshareResponse{}, nil
	}
	return s.C.InitReshare(c, in)
}

// PingPong ...
func (s *DefaultService) PingPong(c context.Context, in *drand.Ping) (*drand.Pong, error) {
	return &drand.Pong{}, nil
}

// Share ...
func (s *DefaultService) Share(c context.Context, in *drand.ShareRequest) (*drand.ShareResponse, error) {
	if s.C == nil {
		return &drand.ShareResponse{}, nil
	}
	return s.C.Share(c, in)
}

// PublicKey ...
func (s *DefaultService) PublicKey(c context.Context, in *drand.PublicKeyRequest) (*drand.PublicKeyResponse, error) {
	if s.C == nil {
		return &drand.PublicKeyResponse{}, nil
	}
	return s.C.PublicKey(c, in)
}

// PrivateKey ...
func (s *DefaultService) PrivateKey(c context.Context, in *drand.PrivateKeyRequest) (*drand.PrivateKeyResponse, error) {
	if s.C == nil {
		return &drand.PrivateKeyResponse{}, nil
	}
	return s.C.PrivateKey(c, in)
}

// CollectiveKey ...
func (s *DefaultService) CollectiveKey(c context.Context, in *drand.CokeyRequest) (*drand.CokeyResponse, error) {
	if s.C == nil {
		return &drand.CokeyResponse{}, nil
	}
	return s.C.CollectiveKey(c, in)
}

// GroupFile  ...
func (s *DefaultService) GroupFile(c context.Context, in *drand.GroupTOMLRequest) (*drand.GroupTOMLResponse, error) {
	if s.C == nil {
		return &drand.GroupTOMLResponse{}, nil
	}
	return s.C.GroupFile(c, in)
}
