package beacon

import "time"

// MaxSyncWaitTime sets how long we'll wait after a new connection to receive new beacons
// from one peer
var MaxSyncWaitTime = 2 * time.Second
