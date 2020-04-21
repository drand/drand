package beacon

import "time"

// Period of time nodes sync with each other just to make sure they are synced
// with each other
var CheckSyncPeriod = 5 * time.Minute

// Once a connection is made, we should not wait too much to receive new beacons
// from one peer
var MaxSyncWaitTime = 2 * time.Second
