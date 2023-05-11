package beacon

import "time"

// MaxSyncWaitTime sets how long we'll wait after a new connection to receive new beacons
// from one peer
var MaxSyncWaitTime = 2 * time.Second

// MaxPartialsPerNode is the maximum number of partials the cache stores about
// any node at any given time. This constant could be much lower, 3 for example
// but when the network is catching up, it may happen that some nodes goes much
// faster than other. In that case, multiple partials can be received from a
// fast nodes and these are valid.
const MaxPartialsPerNode = 100

// MaxCatchupBuffer is the maximum size of the channel that receives beacon from
// a sync mechanism.
const MaxCatchupBuffer = 1000

// CallbackWorkerQueue is the length of the channel that the callback worker
// uses to dispatch beacons to its workers.
const CallbackWorkerQueue = 100
