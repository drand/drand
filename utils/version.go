package utils

import (
	"fmt"

	"github.com/drand/drand/protobuf/common"
)

const (
	FallbackMajor = 0
	FallbackMinor = 0
	FallbackPatch = 0
)

type Version struct {
	Major uint32
	Minor uint32
	Patch uint32
}

func (v Version) IsCompatible(verRcv Version) bool {
	if verRcv.Major == FallbackMajor && verRcv.Minor == FallbackMinor && verRcv.Patch == FallbackPatch {
		return true
	}
	if v.Major == verRcv.Major {
		return true
	}

	return false
}

func (v Version) ToProto() *common.NodeVersion {
	return &common.NodeVersion{Minor: v.Minor, Major: v.Major, Patch: v.Patch}
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}
