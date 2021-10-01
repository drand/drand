package utils

import (
	"fmt"

	"github.com/drand/drand/protobuf/common"
)

const (
	FallbackMayor = 0
	FallbackMinor = 0
	FallbackPatch = 0
)

type Version struct {
	Mayor uint32
	Minor uint32
	Patch uint32
}

func (v Version) IsCompatible(verRcv Version) bool {
	if verRcv.Mayor == FallbackMayor && verRcv.Minor == FallbackMinor && verRcv.Patch == FallbackPatch {
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
