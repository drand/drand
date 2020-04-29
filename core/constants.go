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
const DefaultDKGTimeout = 60 * time.Second

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

// DefaultGenesisOffset is the time that the leader adds to the current time
// to compute the genesis time. It computes the genesis time *before* sending
// the group to the nodes and before running the DKG so it must be sufficiently
// high enough (at the very least superior than DefaultSyncTime + dkg timeout).
var DefaultGenesisOffset = 1 * time.Minute

// DefaultResharingOff is the time the leader adds to the current time to set
// the TransitionTime field in the group file when setting up a resharing. This
// time will be rounded up to the next round time of the beacon, since a beacon
// has to keep the same period.
var DefaultResharingOffset = 30 * time.Second

// Keep the most recents beacons
// XXX unused for now
var DefaultBeaconCacheLength = 10

// DefaultDKGOffset is the default value of of the dkg offset. It's a valud that
// is used to set the time for which nodes should start the DKG.
// To avoid any concurrency / networking effect where nodes start the DKG
// while some others still haven't received the group configuration, the
// coordinator do this in two steps: first, send the group configuration to
// every node, and then every node start at the specified time. This offset
// is set to be sufficiently large such that with high confidence all nodes
// received the group file by then. The coordinator simply does time.Now() +
// DKGOffset.
var DefaultDKGOffset = 5 * time.Second

// IDs for callback when beacon appears
const callbackID = "callbackID"
const cacheID = "cacheID"
