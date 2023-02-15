package common

import (
	"fmt"
	"os"

	pbcommon "github.com/drand/drand/protobuf/common"
)

// Must be manually updated!
// Before releasing: Verify the version number and set Prerelease to ""
// After releasing: Increase the Patch number and set Prerelease to "-pre"
var version = Version{
	Major:      1,
	Minor:      4,
	Patch:      9,
	Prerelease: "",
}

// Set via -ldflags. Example:
//
//	go install -ldflags "-X common.BUILDDATE=`date -u +%d/%m/%Y@%H:%M:%S` -X common.GITCOMMIT=`git rev-parse HEAD`
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
	// This is to get around the problem with the regression test - Prerelease versions are compatible with anything
	if os.Getenv("DISABLE_VERSION_CHECK") == "1" {
		return true
	}

	// Hardcoded the latest potential breakage of network packets.
	// Since v1.4.0 we are now using GRPC deprecation warnings to handle network packet changes.
	switch {
	case v.Major == verRcv.Major && v.Minor == verRcv.Minor:
		return true
	case v.Major == 1 && verRcv.Major == 1 && verRcv.Minor >= 4:
		return true
	case v.Major == 2 && verRcv.Major == 1 && verRcv.Minor >= 5:
		return true
	case v.Major > 1 && v.Major == verRcv.Major:
		return true
	}

	return false
}

func (v Version) ToProto() *pbcommon.NodeVersion {
	return &pbcommon.NodeVersion{Minor: v.Minor, Major: v.Major, Patch: v.Patch}
}

func (v Version) String() string {
	pre := ""
	if v.Prerelease != "" {
		pre = "-"
	}
	return fmt.Sprintf("%d.%d.%d%s%s", v.Major, v.Minor, v.Patch, pre, v.Prerelease)
}
