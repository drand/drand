package common

import (
	"strconv"
	"sync"
	"time"
)

const (
	VersionBase = 10
	VersionSize = 32
)

var LoadProcess sync.Once
var version Version
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

func GetVersionNum() float64 {
	version := GetAppVersion()
	return float64(version.Major)*1000000.0 + float64(version.Minor)*1000.0 + float64(version.Patch)
}

func GetBuildTimestamp() float64 {
	if BUILDDATE == "" {
		return 0.0
	}

	layout := "02/01/2006@15:04:05"
	t, err := time.Parse(layout, BUILDDATE)
	if err != nil {
		return 0.0
	}
	return float64(t.Unix())
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
