package common

import (
	"strconv"

	"github.com/drand/drand/utils"
)

const (
	VersionBase = 10
	VersionSize = 32
)

var (
	MAJOR = "0"
	MINOR = "0"
	PATCH = "0"
)

func GetAppVersion() utils.Version {
	major, err := strconv.ParseInt(MAJOR, VersionBase, VersionSize)
	if err != nil {
		return utils.Version{Major: 0, Minor: 0, Patch: 0}
	}

	minor, err := strconv.ParseInt(MINOR, VersionBase, VersionSize)
	if err != nil {
		return utils.Version{Major: 0, Minor: 0, Patch: 0}
	}

	patch, err := strconv.ParseInt(PATCH, VersionBase, VersionSize)
	if err != nil {
		return utils.Version{Major: 0, Minor: 0, Patch: 0}
	}

	return utils.Version{Major: uint32(major), Minor: uint32(minor), Patch: uint32(patch)}
}
