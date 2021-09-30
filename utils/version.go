package utils

import (
	"fmt"
	"github.com/drand/drand/protobuf/common"
)

const (
	FALLBACK_MAYOR = 0
	FALLBACK_MINOR = 0
	FALLBACK_PATCH = 0
)

type Version struct {
	Mayor uint32
	Minor uint32
	Patch uint32
}

func (v Version) IsCompatible(verRcv Version) bool {
	if verRcv.Mayor == FALLBACK_MAYOR && verRcv.Minor == FALLBACK_MINOR && verRcv.Patch == FALLBACK_PATCH {
		return true
	}
	if v.Mayor == verRcv.Mayor {
		return true
	}

	return false
}

func (v Version) ToProto() *common.NodeVersion {
	return &common.NodeVersion{Minor: v.Minor, Mayor: v.Mayor, Patch: v.Patch}
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Mayor, v.Minor, v.Patch)
}
