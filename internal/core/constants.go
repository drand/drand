package core

import (
	"path"
	"time"

	"github.com/drand/drand/internal/fs"
)

// DefaultConfigFolderName is the name of the folder containing all key materials
// (and the beacons db file by default). It is relative to the user's home
// directory.
const DefaultConfigFolderName = ".drand"

// DefaultConfigFolder returns the default path of the configuration folder.
func DefaultConfigFolder() string {
	return path.Join(fs.HomeFolder(), DefaultConfigFolderName)
}

// DefaultDBFolder is the name of the folder in which the db file is saved.
// It is relative to the DefaultConfigFolder path.
const DefaultDBFolder = "db"

// DefaultControlPort is the default port the daemon and CLI use to communicate together.
const DefaultControlPort = "8888"

// DefaultDKGPhaseTimeout is the default time of each DKG period by default. Note
// that by default, DKG uses the "fast sync" mode that shorten the first phase
// and the second phase, "as fast as possible" when the protocol runs smoothly
// (there is no malicious party).
const DefaultDKGPhaseTimeout = 10 * time.Second

// DefaultDKGKickoffGracePeriod is the amount of time that each node waits after
// receiving the execution notification from the leader.
const DefaultDKGKickoffGracePeriod = 5 * time.Second

// DefaultDKGTimeout is the max amount of time from start of a DKG until it gets aborted automatically
const DefaultDKGTimeout = 24 * time.Hour

const callMaxTimeout = 10 * time.Second
