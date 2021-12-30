package core

import (
	"fmt"
	"github.com/drand/drand/common"

	protoCommon "github.com/drand/drand/protobuf/common"
)

func (dd *DrandDaemon) getBeaconProcess(metadata *protoCommon.Metadata) (*BeaconProcess, string, error) {
	rcvBeaconID := metadata.GetBeaconID()

	if chainHashHex := metadata.GetChainHash(); len(chainHashHex) != 0 {
		chainHash := fmt.Sprintf("%x", chainHashHex)

		dd.state.Lock()
		beaconIDByHash, isChainHashFound := dd.bpByHash[chainHash]
		dd.state.Unlock()

		if isChainHashFound {
			if rcvBeaconID != "" && rcvBeaconID != beaconIDByHash {
				return nil, beaconIDByHash, fmt.Errorf("beacon id [%s] is not running", beaconIDByHash)
			}
			rcvBeaconID = beaconIDByHash
		}
	}

	if rcvBeaconID == "" {
		rcvBeaconID = common.DefaultBeaconID
	}

	dd.state.Lock()
	bp, isBeaconIDFound := dd.bpByID[rcvBeaconID]
	dd.state.Unlock()

	if isBeaconIDFound {
		return bp, rcvBeaconID, nil
	}

	return nil, rcvBeaconID, fmt.Errorf("beacon id [%s] is not running", rcvBeaconID)
}
