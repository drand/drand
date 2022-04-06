package common

import (
	"fmt"

	pbcommon "github.com/drand/drand/protobuf/common"
)

// Must be manually updated!
// Before releasing: Verify the version number and set Prerelease to ""
// After releasing: Increase the Patch number and set Prerelease to "-pre"
var version = Version{
	Major:      1,
	Minor:      4,
	Patch:      0,
	Prerelease: "+pre",
}

// Set via -ldflags. Example:
//   go install -ldflags "-X common.BUILDDATE=`date -u +%d/%m/%Y@%H:%M:%S` -X common.GITCOMMIT=`git rev-parse HEAD`
//
// See the Makefile and the Dockerfile in the root directory of the repo
var (
	COMMIT    = ""
	BUILDDATE = ""
)

func GetAppVersion() Version {
	return version
}

type Version struct {
	Major      uint32
	Minor      uint32
	Patch      uint32
	Prerelease string
}

func (v Version) IsCompatible(verRcv Version) bool {
	// Backwards compatibility with the previous version numbering scheme. We should remove it at some point.
	if verRcv.Major == 0 && verRcv.Minor == 0 && verRcv.Patch == 0 {
		return true
	}
	if v.Major == 0 && v.Minor == 0 && v.Patch == 0 {
		return true
	}

	if v.Major == verRcv.Major && v.Minor == verRcv.Minor {
		return true
	}

	return false
}

func (v Version) ToProto() *pbcommon.NodeVersion {
	return &pbcommon.NodeVersion{Minor: v.Minor, Major: v.Major, Patch: v.Patch}
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d%s", v.Major, v.Minor, v.Patch, v.Prerelease)
}
