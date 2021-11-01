package common

import (
	"strconv"
	"sync"

	"github.com/drand/drand/utils"
)

const (
	VersionBase = 10
	VersionSize = 32
)

var LoadProcess sync.Once
var version utils.Version
var (
	MAJOR = "0"
	MINOR = "0"
	PATCH = "0"
)

func GetAppVersion() utils.Version {
	LoadProcess.Do(parseAppVersion)
	return version
}

func parseAppVersion() {
	major, err := strconv.ParseInt(MAJOR, VersionBase, VersionSize)
	if err != nil {
		version = utils.Version{Major: 0, Minor: 0, Patch: 0}
	}

	minor, err := strconv.ParseInt(MINOR, VersionBase, VersionSize)
	if err != nil {
		version = utils.Version{Major: 0, Minor: 0, Patch: 0}
	}

	patch, err := strconv.ParseInt(PATCH, VersionBase, VersionSize)
	if err != nil {
		version = utils.Version{Major: 0, Minor: 0, Patch: 0}
	}

	version = utils.Version{Major: uint32(major), Minor: uint32(minor), Patch: uint32(patch)}
}
