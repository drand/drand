package core

import (
	"crypto/cipher"
	"crypto/sha256"
	"path"
	"time"

	"github.com/drand/drand/fs"
	"github.com/drand/drand/key"
	"github.com/drand/kyber"
	"github.com/drand/kyber/sign/schnorr"
	"github.com/drand/kyber/util/random"
)

// DefaultConfigFolderName is the name of the folder containing all key materials
// (and the beacons db file by default). It is relative to the user's home
// directory.
const DefaultConfigFolderName = ".drand"

// DefaultConfigFolder returns the default path of the configuration folder.
func DefaultConfigFolder() string {
	return path.Join(fs.HomeFolder(), DefaultConfigFolderName)
}

// DefaultDBFolder is the name of the folder in which the db file is saved. By
// default it is relative to the DefaultConfigFolder path.
const DefaultDBFolder = "db"

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

// EciesHash is the hash function used for the ECIES encryption used in the
// private randomness feature.
var EciesHash = sha256.New

// MaxWaitPrepareDKG is the maximum time the "automatic" setting up of the group
// can take. If the setup is still not finished after this time, it is
// canceled.
var MaxWaitPrepareDKG = 24 * 7 * 2 * time.Hour

const genesisOffsetTimeoutMultiple = 3

// DefaultGenesisOffset is the time that the leader adds to the current time to
// compute the genesis time. It computes the genesis time *before* sending the
// group to the nodes and before running the DKG so it must be sufficiently high
// enough (at the very least superior than the time DKG is taking).
const DefaultGenesisOffset = DefaultDKGTimeout * time.Duration(genesisOffsetTimeoutMultiple)

// DefaultResharingOffset is the time the leader adds to the current time to set
// the TransitionTime field in the group file when setting up a resharing. This
// time will be rounded up to the next round time of the beacon, since a beacon
// has to keep the same period.
var DefaultResharingOffset = 30 * time.Second

// PrivateRandLength is the length of expected private randomness buffers
const PrivateRandLength = 32

// DKGAuthScheme is the signature scheme used to authentify packets during
// a broadcast
var DKGAuthScheme = schnorr.NewScheme(&schnorrSuite{key.KeyGroup})

type schnorrSuite struct {
	kyber.Group
}

func (s *schnorrSuite) RandomStream() cipher.Stream {
	return random.New()
}
