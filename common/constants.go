package common

import (
	"strconv"
	"sync"
)

const (
	VersionBase = 10
	VersionSize = 32
)

var LoadProcess sync.Once
var version Version

// Set via -ldflags
// Example (it should all be one line):
//   go install -ldflags
//     "-X common.BUILDDATE=`date -u +%d/%m/%Y@%H:%M:%S`
//      -X common.GITCOMMIT=`git rev-parse HEAD`
//      -X common.MAJOR=1
//      -X common.MINOR=2
//      -X common.PATCH=3"
// See the Makefile and the Dockerfile in the root directory of the repo
var (
	MAJOR     = "0"
	MINOR     = "0"
	PATCH     = "0"
	COMMIT    = ""
	BUILDDATE = ""
)

func GetAppVersion() Version {
	LoadProcess.Do(parseAppVersion)
	return version
}

func parseAppVersion() {
	major, err := strconv.ParseInt(MAJOR, VersionBase, VersionSize)
	if err != nil {
		version = Version{Major: 0, Minor: 0, Patch: 0}
	}

	minor, err := strconv.ParseInt(MINOR, VersionBase, VersionSize)
	if err != nil {
		version = Version{Major: 0, Minor: 0, Patch: 0}
	}

	patch, err := strconv.ParseInt(PATCH, VersionBase, VersionSize)
	if err != nil {
		version = Version{Major: 0, Minor: 0, Patch: 0}
	}

	version = Version{Major: uint32(major), Minor: uint32(minor), Patch: uint32(patch)}
}
