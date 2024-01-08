package pgdb

import (
	"github.com/drand/drand/v2/common"
)

// dbBeacon represents a beacon that is stored in the database.
type dbBeacon struct {
	PreviousSig []byte `db:"previous_sig,omitempty"`
	Round       uint64 `db:"round"`
	Signature   []byte `db:"signature"`
}

func toChainBeacon(dbB dbBeacon) *common.Beacon {
	cb := common.Beacon(dbB)
	return &cb
}
