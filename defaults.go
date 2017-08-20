package main

import (
	"os"
	"path"
	"strings"
)

const defaultKeyFile = "drand_id"
const privateExtension = ".private"
const publicExtension = ".public"
const defaultGroupFile_ = "drand_group.toml"
const defaultShareFile_ = "drand_share.secret"

// default threshold for the distributed key generation protocol & TBLS.
func defaultThreshold(n int) int {
	return n * 2 / 3
}

func defaultPrivateFile() string {
	return path.Join(pwd(), defaultKeyFile+privateExtension)
}

func publicFile(privateFile string) string {
	ss := strings.Split(privateFile, privateExtension)
	return ss[0] + publicExtension
}

func defaultGroupFile() string {
	return path.Join(pwd(), defaultGroupFile_)
}

func defaultShareFile() string {
	return path.Join(pwd(), defaultShareFile_)
}

func pwd() string {
	s, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return s
}
