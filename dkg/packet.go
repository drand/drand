package dkg

import "github.com/dedis/kyber/share/dkg/pedersen"

// Packet holds any message exchanged during a DKG protocol.
type Packet struct {
	Deal          *dkg.Deal
	Response      *dkg.Response
	Justification *dkg.Justification
}
