package main

import (
	"os"
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
	return pwd() + defaultKeyFile + privateExtension
}

func publicFile(privateFile string) string {
	ss := strings.Split(privateFile, privateExtension)
	return ss[0] + publicExtension
}

func defaultGroupFile() string {
	return pwd() + defaultGroupFile_
}

func defaultShareFile() string {
	return pwd() + defaultShareFile_
}

func pwd() string {
	s, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return s
}
