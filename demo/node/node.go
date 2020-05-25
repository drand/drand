package node

import (
	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
)

type Node interface {
	Start(certFolder string) error
	PrivateAddr() string
	PublicAddr() string
	Index() int
	RunDKG(nodes, thr int, timeout string, leader bool, leaderAddr string, beaconOffset int) *key.Group
	GetGroup() *key.Group
	RunReshare(nodes, thr int, oldGroup string, timeout string, leader bool, leaderAddr string, beaconOffset int) *key.Group
	ChainInfo(group string) bool
	Ping() bool
	GetBeacon(groupPath string, round uint64) (*drand.PublicRandResponse, string)
	WriteCertificate(path string)
	WritePublic(path string)
	Stop()
	PrintLog()
}
