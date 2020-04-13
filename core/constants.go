package core

import (
	"crypto/sha256"
	"path"
	"time"

	"github.com/drand/drand/fs"
)

// DefaultConfigFolderName is the name of the folder containing all key materials
// (and the beacons db file by default). It is relative to the user's home
// directory.
const DefaultConfigFolderName = ".drand"

// DefaultConfigFolder returns the default path of the configuration folder.
func DefaultConfigFolder() string {
	return path.Join(fs.HomeFolder(), DefaultConfigFolderName)
}

// DefaultDbFolder is the name of the folder in which the db file is saved. By
// default it is relative to the DefaultConfigFolder path.
const DefaultDbFolder = "db"

// DefaultBeaconPeriod is the period in which the beacon logic creates new
// random beacon.
const DefaultBeaconPeriod time.Duration = 1 * time.Minute

// DefaultControlPort is the default port the functionnality control port communicate on.
const DefaultControlPort = "8888"

// DefaultDKGTimeout is the time the DKG timeouts by default. See
// kyber/share/dkg/pedersen for more information.
const DefaultDKGTimeout = "1m"

// DefaultDialTimeout is the timeout given to gRPC when dialling a remote server
var DefaultDialTimeout = 10 * time.Second

// RandomnessHash is the hash function used to produce the final randomness from
// the signature
var RandomnessHash = sha256.New

// DefaultWaitTime is the time beacon nodes wait before asking other nodes for
// partial signature. Because time shifts can happen
var DefaultWaitTime = 300 * time.Millisecond

// DefaultBeaconOffset is the default minimum time to wait form the time the DKG
// is launched to the time the beacon chain starts.
var DefaultBeaconOffset = time.Duration(2*60) * time.Second

// MaxWaitPrepareDKG is the maximum time the "automatic" setting up of the group
// can take. If the setup is still not finished after this time, it is
// cancelled.
var MaxWaitPrepareDKG = 24 * 7 * 2 * time.Hour

// DefaultSyncTime is the time the leader waits after sending the group file to
// all participants. It gives a bit of time to make sure every node has received
// the group file and launched their DKG. Since it is not a time critical
// process, we can afford to wait here.
var DefaultSyncTime = 5 * time.Second

// DefaultGenesisOffset is the time that the leader adds to the current time
// to compute the genesis time. It computes the genesis time *before* sending
// the group to the nodes and before running the DKG so it must be sufficiently
// high enough (at the very least superior than DefaultSyncTime + dkg timeout).
var DefaultGenesisOffset = 2 * time.Minute

// DefaultResharingOff is the time the leader adds to the current time to set
// the TransitionTime field in the group file when setting up a resharing. This
// time will be rounded up to the next round time of the beacon, since a beacon
// has to keep the same period.
var DefaultResharingOffset = 30 * time.Second

// Keep the most recents beacons
var DefaultBeaconCacheLength = 10

// IDs for callback when beacon appears
const callbackID = "callbackID"
const cacheID = "cacheID"
