package core

import (
	"github.com/drand/drand/protobuf/drand"
)

type DKGMapping[T any, U any] interface {
	Enrich(T) (U, error)
	Apply(U, *DKGDetails) (*DKGDetails, DKGErrorCode)
}

type FirstProposalMapping struct {
	me *drand.Participant
}

func (p FirstProposalMapping) Enrich(incomingPacket *drand.FirstProposalOptions) (*ProposalTerms, error) {
	return &ProposalTerms{
		BeaconID:  incomingPacket.BeaconID,
		Threshold: incomingPacket.Threshold,
		Epoch:     1,
		Timeout:   incomingPacket.Timeout.AsTime(),
		Leader:    p.me,
		Joining:   incomingPacket.Joining,
		Remaining: nil,
		Leaving:   nil,
	}, nil
}

func (p FirstProposalMapping) Apply(terms *ProposalTerms, currentState *DKGDetails) (*DKGDetails, DKGErrorCode) {
	return currentState.Proposing(p.me, terms)
}

type ProposalMapping struct {
	me *drand.Participant
}

func (p ProposalMapping) Enrich(options *drand.ProposalOptions) (*ProposalTerms, error) {
	return &ProposalTerms{
		BeaconID:  options.BeaconID,
		Threshold: options.Threshold,
		Epoch:     2,
		Timeout:   options.Timeout.AsTime(),
		Leader:    p.me,
		Joining:   options.Joining,
		Remaining: options.Remaining,
		Leaving:   options.Leaving,
	}, nil
}

func (p ProposalMapping) Apply(terms *ProposalTerms, currentState *DKGDetails) (*DKGDetails, DKGErrorCode) {
	return currentState.Proposing(p.me, terms)
}
