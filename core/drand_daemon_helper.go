package core

import (
	"fmt"

	"github.com/drand/drand/common/constants"
	"github.com/drand/drand/protobuf/common"
)

func (dd *DrandDaemon) getBeaconProcess(metadata *common.Metadata) (*BeaconProcess, error) {
	beaconID := ""
	if beaconID = metadata.GetBeaconID(); beaconID == "" {
		beaconID = constants.DefaultBeaconID
	}

	if _, isBeaconRunning := dd.beaconProcesses[beaconID]; isBeaconRunning {
		return nil, fmt.Errorf("beacon id is not running")
	}

	bp, _ := dd.beaconProcesses[beaconID]
	return bp, nil
}
