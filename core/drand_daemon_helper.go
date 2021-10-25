package core

import (
	"fmt"

	"github.com/drand/drand/common"
	protoCommon "github.com/drand/drand/protobuf/common"
)

func (dd *DrandDaemon) getBeaconProcess(metadata *protoCommon.Metadata) (*BeaconProcess, string, error) {
	beaconID := ""
	if beaconID = metadata.GetBeaconID(); beaconID == "" {
		beaconID = common.DefaultBeaconID
	}

	dd.state.Lock()
	bp, isBpCreated := dd.beaconProcesses[beaconID]
	dd.state.Unlock()

	if !isBpCreated {
		return nil, beaconID, fmt.Errorf("beacon id [%s] is not running", beaconID)
	}

	return bp, beaconID, nil
}
