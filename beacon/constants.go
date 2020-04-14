package beacon

import "time"

// SycnRetrialWait denotes how much time a node waits after a sync that didn't
// give a beacon of the "current" round before retrying to sync.
var SyncRetrialWait = 2 * time.Second

// How much time a node tries to sync before running the beacon on what he has
// best.
var SyncRetrial = 1
