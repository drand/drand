package core

import (
	"path"
	"time"

	"github.com/dedis/drand/fs"
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
const DefaultDKGTimeout = "10s"
