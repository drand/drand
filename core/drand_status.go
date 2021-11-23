package core

import (
	"fmt"
	"strings"

	"github.com/drand/drand/protobuf/drand"
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

type ReshareStatus uint32

const (
	ReshareNotInProgress ReshareStatus = iota
	ReshareInProgress
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

func GetReshareStatusDescription(value ReshareStatus) string {
	switch value {
	case ReshareInProgress:
		return InProgressDesc
	case ReshareNotInProgress:
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
	reshareStatus := GetReshareStatusDescription(ReshareStatus(status.Reshare.Status))
	beaconStatus := GetBeaconDescription(BeaconStatus(status.Beacon.Status))

	output := new(strings.Builder)
	fmt.Fprintf(output, "* Dkg \n")
	fmt.Fprintf(output, " - Status: %s \n", dkgStatus)
	fmt.Fprintf(output, "* Reshare \n")
	fmt.Fprintf(output, " - Status: %s \n", reshareStatus)
	fmt.Fprintf(output, "* ChainStore \n")
	fmt.Fprintf(output, " - IsEmpty: %t \n", status.ChainStore.IsEmpty)
	fmt.Fprintf(output, " - LastRound: %d \n", status.ChainStore.LastRound)
	fmt.Fprintf(output, "* Beacons \n")
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
