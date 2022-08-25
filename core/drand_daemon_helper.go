package core

import (
	"fmt"

	"github.com/drand/drand/common"
	protoCommon "github.com/drand/drand/protobuf/common"
)

func (dd *DrandDaemon) readBeaconID(metadata *protoCommon.Metadata) (string, error) {
	rcvBeaconID := metadata.GetBeaconID()

	if chainHashHex := metadata.GetChainHash(); len(chainHashHex) != 0 {
		chainHash := fmt.Sprintf("%x", chainHashHex)

		dd.state.Lock()
		beaconIDByHash, isChainHashFound := dd.chainHashes[chainHash]
		dd.state.Unlock()
		if isChainHashFound {
			// check if rcv beacon id on request points to a different id obtained from chain hash
			// we accept the empty beacon id, since we do a match on the chain hash in that case
			if rcvBeaconID != "" && !common.CompareBeaconIDs(rcvBeaconID, beaconIDByHash) {
				return "", fmt.Errorf("invalid chain hash: '%s' != '%s'", rcvBeaconID, beaconIDByHash)
			}
			rcvBeaconID = beaconIDByHash
		} else {
			// for the case where our node is still waiting for the chain hash to be set
			rcvBeaconID = common.GetCanonicalBeaconID(rcvBeaconID)
			for id, bp := range dd.beaconProcesses {
				// we only accept to proceed with an unknown chain hash if one beacon process hasn't run DKG yet
				if id == rcvBeaconID && bp.group == nil {
					// we make sure that the beacon id is not empty
					metadata.BeaconID = rcvBeaconID
					return id, nil
				}
			}
			// if no beacon process is not yet initialized entirely, this is an error
			return "", fmt.Errorf("unknown chain hash: %s out of %v", chainHash, dd.chainHashes)
		}
	}

	// if we didn't match on a chain hash, and have the empty string, then it's the default beacon
	rcvBeaconID = common.GetCanonicalBeaconID(rcvBeaconID)
	// make sure the metadata use a correct beacon id
	if metadata == nil {
		metadata = &protoCommon.Metadata{}
	}
	// we explicitly set the beacon id on the metadata in case it changed because of GetCanonicalBeaconID
	metadata.BeaconID = rcvBeaconID

	return rcvBeaconID, nil
}

func (dd *DrandDaemon) getBeaconProcessByID(beaconID string) (*BeaconProcess, error) {
	dd.state.Lock()
	bp, isBeaconIDFound := dd.beaconProcesses[beaconID]
	dd.state.Unlock()

	if isBeaconIDFound {
		return bp, nil
	}

	return nil, fmt.Errorf("beacon id [%s] is not running", beaconID)
}

func (dd *DrandDaemon) getBeaconProcessFromRequest(metadata *protoCommon.Metadata) (*BeaconProcess, error) {
	beaconID, err := dd.readBeaconID(metadata)
	if err != nil {
		return nil, err
	}

	return dd.getBeaconProcessByID(beaconID)
}
