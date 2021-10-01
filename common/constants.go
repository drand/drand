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
	MAYOR = "0"
	MINOR = "0"
	PATCH = "0"
)

func GetAppVersion() utils.Version {
	mayor, err := strconv.ParseInt(MAYOR, VersionBase, VersionSize)
	if err != nil {
		return utils.Version{Mayor: 0, Minor: 0, Patch: 0}
	}

	minor, err := strconv.ParseInt(MINOR, VersionBase, VersionSize)
	if err != nil {
		return utils.Version{Mayor: 0, Minor: 0, Patch: 0}
	}

	patch, err := strconv.ParseInt(PATCH, VersionBase, VersionSize)
	if err != nil {
		return utils.Version{Mayor: 0, Minor: 0, Patch: 0}
	}

	return utils.Version{Mayor: uint32(mayor), Minor: uint32(minor), Patch: uint32(patch)}
}
