package core

import (
	"context"
	"github.com/drand/drand/protobuf/drand"
)

func (d *DKGProcess) Accept(_ context.Context, acceptance *drand.AcceptProposal) (*drand.GenericResponseMessage, error) {
	return err(), nil
}

func (d *DKGProcess) Reject(_ context.Context, rejection *drand.RejectProposal) (*drand.GenericResponseMessage, error) {
	return err(), nil
}

func (d *DKGProcess) SendError(_ context.Context, error *drand.DKGError) (*drand.GenericResponseMessage, error) {
	return err(), nil
}

func (d *DKGProcess) Propose(_ context.Context, proposal *drand.Proposal) (*drand.GenericResponseMessage, error) {
	return err(), nil
}

func (d *DKGProcess) Abort(_ context.Context, abort *drand.AbortDKG) (*drand.GenericResponseMessage, error) {
	return err(), nil
}

func (d *DKGProcess) Execute(_ context.Context, kickoff *drand.StartExecution) (*drand.GenericResponseMessage, error) {
	return err(), nil
}

func err() *drand.GenericResponseMessage {
	return &drand.GenericResponseMessage{
		IsError:      true,
		ErrorMessage: "this call has not yet been implemented for this service",
		ErrorCode:    1,
	}
}
