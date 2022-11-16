package core

import (
	"fmt"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
)

type DKGProtocolSteps[CmdIn any, ProtocolIn any, ProtocolOut any] interface {
	// Enrich takes a CLI message and enriches it with the necessary information to be applied to the DKG state
	Enrich(CmdIn) (ProtocolIn, error)

	// Apply takes a protocol input message (enriched from the CLI input) and the current state of the DKG from the database
	// and applies it to create the next state (which should be saved to the database), or an error if something goes wrong
	Apply(ProtocolIn, *DKGDetails) (*DKGDetails, error)

	// Responses takes the protocol input message and updated DKG state and creates the required network responses
	// that need to be made (generally either to a single party or all of the parties in the DKG
	Responses(ProtocolIn, *DKGDetails) ([]*NetworkResponse[ProtocolOut], error)

	// ForwardResponse takes a DKG client and one of the network responses from Responses and executes the required
	// protocol networking step for it
	ForwardResponse(client drand.DKGClient, response *NetworkResponse[ProtocolOut]) error
}

// executeProtocolSteps performs the mapping and state transitions for given DKG packet
// and updates the data store accordingly
func executeProtocolSteps[CmdIn any, ProtocolIn any, ProtocolOut any](
	d *DKGProcess,
	beaconID string,
	protocol DKGProtocolSteps[CmdIn, ProtocolIn, ProtocolOut],
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
	responses, err := protocol.Responses(payload, nextDKGState)
	if err != nil {
		return err
	}

	// send all these responses
	for _, r := range responses {
		client, err := net.NewDKGClient(r.to.Address)
		if err != nil {
			return err
		}
		fmt.Printf("sending response to %s", r.to)
		err = protocol.ForwardResponse(client, r)
		if err != nil {
			return err
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
