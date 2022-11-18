package dkg

import (
	"fmt"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
)

// ProtocolSteps is an abstract representation of the steps taken for any phase in the DKG
// a command comes in, the command is enriched into a message to be applied to the DKG state,
// that message is applied to the current DKG state, the output state is saved to the data store and any
// resulting network requests are made to other relevant parties in the protocol
type ProtocolSteps[CmdIn any, ProtocolIn any, ProtocolOut any] interface {
	// Enrich takes a CLI message and enriches it with the necessary information to be applied to the DKG state
	Enrich(CmdIn) (ProtocolIn, error)

	// Apply takes a protocol input message (enriched from the CLI input) and the current state of the DKG from the database
	// and applies it to create the next state (which should be saved to the database), or an error if something goes wrong
	Apply(ProtocolIn, *DKGState) (*DKGState, error)

	// Requests takes the protocol input message and updated DKG state and creates the required network responses
	// that need to be made (generally either to a single party or all of the parties in the DKG
	Requests(ProtocolIn, *DKGState) ([]*NetworkRequest[ProtocolOut], error)

	// ForwardRequest takes a DKG client and one of the network responses from Responses and executes the required
	// protocol networking step for it
	ForwardRequest(client drand.DKGClient, request *NetworkRequest[ProtocolOut]) error
}

// executeProtocolSteps performs the mapping and state transitions for given DKG packet
// and updates the data store accordingly
func executeProtocolSteps[CmdIn any, ProtocolIn any, ProtocolOut any](
	d *DKGProcess,
	beaconID string,
	protocol ProtocolSteps[CmdIn, ProtocolIn, ProtocolOut],
	inputPacket CmdIn,
) error {

	// remap the CLI payload into one useful for DKG state
	payload, err := protocol.Enrich(inputPacket)
	if err != nil {
		return err
	}

	// pull the latest DKG state from the database
	currentDKGState, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return err
	}

	// apply our enriched DKG payload onto the current DKG state to create a new state
	nextDKGState, err := protocol.Apply(payload, currentDKGState)
	if err != nil {
		return err
	}

	// create any responses that we need to send out to other nodes.
	// Likely this will be to either a single node or all the nodes involved
	responses, err := protocol.Requests(payload, nextDKGState)
	if err != nil {
		return err
	}

	// send all the responses to relevant nodes in the network
	for _, r := range responses {
		client, err := net.NewDKGClient(r.to.Address)
		if err != nil {
			return err
		}
		d.log.Debugw("sending DKG message", "to", r.to.Address)
		err = protocol.ForwardRequest(client, r)
		if err != nil {
			return fmt.Errorf("error from %s: %s", r.to.Address, err)
		}
	}

	// save the new state
	if nextDKGState.State == Complete {
		err = d.store.SaveFinished(beaconID, nextDKGState)
	} else {
		err = d.store.SaveCurrent(beaconID, nextDKGState)
	}
	if err != nil {
		return err
	}

	return nil
}
