package common

import (
	"github.com/drand/drand/utils"
	"strconv"
)

var (
	MAYOR = "0"
	MINOR = "0"
	PATCH = "0"
)

func GetAppVersion() utils.Version {
	mayor, err := strconv.ParseInt(MAYOR, 10, 32)
	minor, err := strconv.ParseInt(MINOR, 10, 32)
	patch, err := strconv.ParseInt(PATCH, 10, 32)

	if err != nil {
		return utils.Version{Mayor: 0, Minor: 0, Patch: 0}
	}

	return utils.Version{Mayor: uint32(mayor), Minor: uint32(minor), Patch: uint32(patch)}
}
