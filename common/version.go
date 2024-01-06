package common

import (
	"fmt"
	"os"

	pbcommon "github.com/drand/drand/protobuf/drand"
)

// Must be manually updated!
// Before releasing: Verify the version number and set Prerelease to ""
// After releasing: Increase the Patch number and set Prerelease to "-pre"
var version = Version{
	Major:      2,
	Minor:      0,
	Patch:      0,
	Prerelease: "testnet",
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

	// We are using GRPC deprecation warnings to handle network packet changes to avoid bumping minor too often.
	switch {
	// we always keep retro-compatibility with the immediate minors predecessor
	case v.Major == verRcv.Major && verRcv.Minor+1 >= v.Minor && v.Minor+1 >= verRcv.Minor:
		return true
	case v.Major == 1 && v.Minor >= 5 && v.Patch >= 8 && verRcv.Major == 2 && verRcv.Minor == 0:
		return true
	case v.Major == 2 && v.Minor == 0 && verRcv.Major == 1 && verRcv.Minor >= 5 && verRcv.Patch >= 8:
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
