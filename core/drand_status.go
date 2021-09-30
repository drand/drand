package core

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
