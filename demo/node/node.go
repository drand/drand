package node

import (
	"time"

	"github.com/drand/drand/common/key"
	"github.com/drand/drand/internal/chain"
	pdkg "github.com/drand/drand/protobuf/dkg"
	"github.com/drand/drand/protobuf/drand"
)

type Node interface {
	Start(dbEngineType chain.StorageType, pgDSN func() string, memDBSize int) error
	PrivateAddr() string
	CtrlAddr() string
	PublicAddr() string
	Index() int
	StartLeaderDKG(thr int, catchupPeriod int, joiners []*pdkg.Participant) error
	StartLeaderReshare(thr int, transitionTime time.Time, catchupPeriod int, joiners []*pdkg.Participant, remainers []*pdkg.Participant, leavers []*pdkg.Participant) error
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
	Identity() (*pdkg.Participant, error)
	Stop()
	PrintLog()
}
