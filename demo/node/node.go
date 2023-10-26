package node

import (
	"time"

	"github.com/drand/drand/common/key"
	"github.com/drand/drand/internal/chain"
	"github.com/drand/drand/protobuf/drand"
)

type Node interface {
	Start(dbEngineType chain.StorageType, pgDSN func() string, memDBSize int) error
	PrivateAddr() string
	CtrlAddr() string
	PublicAddr() string
	Index() int
	StartLeaderDKG(thr int, catchupPeriod int, joiners []*drand.Participant) error
	StartLeaderReshare(thr int, transitionTime time.Time, catchupPeriod int, joiners []*drand.Participant, remainers []*drand.Participant, leavers []*drand.Participant) error
	ExecuteLeaderDKG() error
	ExecuteLeaderReshare() error
	JoinDKG() error
	AcceptReshare() error
	JoinReshare(oldGroup key.Group) error
	WaitDKGComplete(epoch uint32, timeout time.Duration) (*key.Group, error)
	GetGroup() *key.Group
	ChainInfo(group string) bool
	Ping() bool
	GetBeacon(groupPath string, round uint64) (*drand.PublicRandResponse, string)
	WritePublic(path string)
	Identity() (*drand.Participant, error)
	Stop()
	PrintLog()
}
