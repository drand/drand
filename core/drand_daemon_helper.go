package core

import (
	"fmt"

	"github.com/drand/drand/common/constants"
	"github.com/drand/drand/protobuf/common"
)

func (dd *DrandDaemon) getBeaconProcess(metadata *common.Metadata) (*BeaconProcess, string, error) {
	beaconID := ""
	if beaconID = metadata.GetBeaconID(); beaconID == "" {
		beaconID = constants.DefaultBeaconID
	}

	dd.state.Lock()
	bp, isBeaconRunning := dd.beaconProcesses[beaconID]
	dd.state.Unlock()

	if !isBeaconRunning {
		return nil, "", fmt.Errorf("beacon id is not running")
	}

	return bp, beaconID, nil
}
