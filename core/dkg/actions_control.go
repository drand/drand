package dkg

import (
	"context"
	"errors"
	"github.com/drand/drand/protobuf/drand"
)

type NetworkRequest[Payload any] struct {
	to      *drand.Participant
	payload Payload
}

type FirstProposalSteps struct {
	me *drand.Participant
}

func (p FirstProposalSteps) Enrich(incomingPacket *drand.FirstProposalOptions) (*drand.ProposalTerms, error) {
	return &drand.ProposalTerms{
		BeaconID:  incomingPacket.BeaconID,
		Threshold: incomingPacket.Threshold,
		Epoch:     1,
		Timeout:   incomingPacket.Timeout,
		Leader:    p.me,
		Joining:   incomingPacket.Joining,
		Remaining: nil,
		Leaving:   nil,
	}, nil
}

func (p FirstProposalSteps) Apply(terms *drand.ProposalTerms, currentState *DKGState) (*DKGState, error) {
	return currentState.Proposing(p.me, terms)
}

func (p FirstProposalSteps) Requests(terms *drand.ProposalTerms, details *DKGState) ([]*NetworkRequest[*drand.Proposal], error) {
	proposal := &drand.Proposal{
		BeaconID:  terms.BeaconID,
		Leader:    terms.Leader,
		Epoch:     terms.Epoch,
		Threshold: terms.Threshold,
		Timeout:   terms.Timeout,
		Joining:   terms.Joining,
		Remaining: terms.Remaining,
		Leaving:   terms.Leaving,
	}

	var requests []*NetworkRequest[*drand.Proposal]

	for _, joiner := range details.Joining {
		if joiner.Address == p.me.Address {
			continue
		}
		requests = append(requests, &NetworkRequest[*drand.Proposal]{
			to:      joiner,
			payload: proposal,
		})
	}
	return requests, nil
}

func (p FirstProposalSteps) ForwardRequest(client drand.DKGClient, networkCall *NetworkRequest[*drand.Proposal]) error {
	response, err := client.Propose(context.Background(), networkCall.payload)
	if err != nil {
		return err
	}

	if response.IsError {
		return errors.New(response.ErrorMessage)
	}

	return nil
}

type ProposalSteps struct {
	me    *drand.Participant
	store DKGStore
}

func (p ProposalSteps) Enrich(options *drand.ProposalOptions) (*drand.ProposalTerms, error) {
	current, err := p.store.GetCurrent(options.BeaconID)
	if err != nil {
		return nil, err
	}
	return &drand.ProposalTerms{
		BeaconID:  options.BeaconID,
		Threshold: options.Threshold,
		Epoch:     current.Epoch + 1,
		Timeout:   options.Timeout,
		Leader:    p.me,
		Joining:   options.Joining,
		Remaining: options.Remaining,
		Leaving:   options.Leaving,
	}, nil
}

func (p ProposalSteps) Apply(terms *drand.ProposalTerms, currentState *DKGState) (*DKGState, error) {
	return currentState.Proposing(p.me, terms)
}

func (p ProposalSteps) Requests(terms *drand.ProposalTerms, details *DKGState) ([]*NetworkRequest[*drand.Proposal], error) {
	proposal := &drand.Proposal{
		BeaconID:  terms.BeaconID,
		Leader:    terms.Leader,
		Epoch:     terms.Epoch,
		Threshold: terms.Threshold,
		Timeout:   terms.Timeout,
		Joining:   terms.Joining,
		Remaining: terms.Remaining,
		Leaving:   terms.Leaving,
	}

	var requests []*NetworkRequest[*drand.Proposal]

	for _, joiner := range details.Joining {

		requests = append(requests, &NetworkRequest[*drand.Proposal]{
			to:      joiner,
			payload: proposal,
		})
	}

	for _, remainer := range details.Remaining {
		if remainer.Address == p.me.Address {
			continue
		}
		requests = append(requests, &NetworkRequest[*drand.Proposal]{
			to:      remainer,
			payload: proposal,
		})
	}

	return requests, nil
}

func (p ProposalSteps) ForwardRequest(client drand.DKGClient, networkCall *NetworkRequest[*drand.Proposal]) error {
	response, err := client.Propose(context.Background(), networkCall.payload)
	if err != nil {
		return err
	}

	if response.IsError {
		return errors.New(response.ErrorMessage)
	}

	return nil
}
