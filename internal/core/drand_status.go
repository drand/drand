package core

import (
	"fmt"
	"strings"

	"github.com/drand/drand/v2/protobuf/drand"
)

const UnknownDesc = "Unknown"
const NotStartedDesc = "Not started"
const InProgressDesc = "In progress"

type DkgStatus uint32

const (
	DkgReady DkgStatus = iota
	DkgInProgress
	DkgNotStarted
)

type BeaconStatus uint32

const (
	BeaconNotInited BeaconStatus = iota
	BeaconInited
)

func GetDkgStatusDescription(value DkgStatus) string {
	switch value {
	case DkgReady:
		return "Done"
	case DkgInProgress:
		return InProgressDesc
	case DkgNotStarted:
		return NotStartedDesc
	default:
		return UnknownDesc
	}
}

func GetBeaconDescription(value BeaconStatus) string {
	switch value {
	case BeaconNotInited:
		return "Not inited"
	case BeaconInited:
		return "Inited"
	default:
		return UnknownDesc
	}
}

func StatusResponseToString(status *drand.StatusResponse) string {
	dkgStatus := GetDkgStatusDescription(DkgStatus(status.Dkg.Status))
	beaconStatus := GetBeaconDescription(BeaconStatus(status.Beacon.Status))

	output := new(strings.Builder)
	fmt.Fprintf(output, "* Dkg \n")
	fmt.Fprintf(output, " - Status: %s \n", dkgStatus)
	fmt.Fprintf(output, "DKG epoch: %d \n", status.Epoch)
	fmt.Fprintf(output, "* ChainStore \n")
	fmt.Fprintf(output, " - IsEmpty: %t \n", status.ChainStore.IsEmpty)
	fmt.Fprintf(output, " - LastRound: %d \n", status.ChainStore.LastStored)
	fmt.Fprintf(output, "* BeaconProcess \n")
	fmt.Fprintf(output, " - Status: %s \n", beaconStatus)
	fmt.Fprintf(output, " - Stopped: %t \n", status.Beacon.IsStopped)
	fmt.Fprintf(output, " - Started: %t \n", status.Beacon.IsStarted)
	fmt.Fprintf(output, " - Serving: %t \n", status.Beacon.IsServing)
	fmt.Fprintf(output, " - Running: %t \n", status.Beacon.IsRunning)
	if conns := status.GetConnections(); len(conns) > 0 {
		fmt.Fprintf(output, "* Network visibility\n")
		for addr, ok := range conns {
			if ok {
				fmt.Fprintf(output, " - %s -> OK\n", addr)
			} else {
				fmt.Fprintf(output, " - %s -> X no connection\n", addr)
			}
		}
	}
	return output.String()
}
