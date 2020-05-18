package beacon

import "time"

// Once a connection is made, we should not wait too much to receive new beacons
// from one peer
var MaxSyncWaitTime = 2 * time.Second
