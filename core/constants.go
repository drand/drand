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

// DefaultDKGTimeout is the default time of each DKG period by default. Note
// that by default, DKG uses the "fast sync" mode that shorten the first phase
// and the second phase, "as fast as possible" when the protocol runs smoothly
// (there is no malicious party).
const DefaultDKGTimeout = 10 * time.Second

// DefaultDialTimeout is the timeout given to gRPC when dialling a remote server
var DefaultDialTimeout = 10 * time.Second

// RandomnessHash is the hash function used to produce the final randomness from
// the signature. NOTE: this is a proposition by drand but user can choose any
// secure hash function to derive the final randomness from the sig:
// rand = H(sig)
var RandomnessHash = sha256.New

// EciesHash is the hash function used for the ECIES encryption used in the
// private randomness feature.
var EciesHash = sha256.New

// MaxWaitPrepareDKG is the maximum time the "automatic" setting up of the group
// can take. If the setup is still not finished after this time, it is
// cancelled.
var MaxWaitPrepareDKG = 24 * 7 * 2 * time.Hour

// DefaultGenesisOffset is the time that the leader adds to the current time to
// compute the genesis time. It computes the genesis time *before* sending the
// group to the nodes and before running the DKG so it must be sufficiently high
// enough (at the very least superior than the time DKG is taking).
var DefaultGenesisOffset = DefaultDKGTimeout * time.Duration(3)

// DefaultResharingOff is the time the leader adds to the current time to set
// the TransitionTime field in the group file when setting up a resharing. This
// time will be rounded up to the next round time of the beacon, since a beacon
// has to keep the same period.
var DefaultResharingOffset = 30 * time.Second

// Keep the most recents beacons
// XXX unused for now
var DefaultBeaconCacheLength = 10

// IDs for callback when beacon appears
const callbackID = "callbackID"
const cacheID = "cacheID"
