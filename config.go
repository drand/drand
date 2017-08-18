package main

import (
	"os"
)

const privateFileName = "private.toml"
const publicFileName = "public.toml"
const groupFileName = "group.toml"

func privateFile() string {
	return pwd() + privateFileName
}

func publicFile() string {
	return pwd() + publicFileName
}

func groupFile() string {
	return pwd() + groupFileName
}

func pwd() string {
	s, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return s
}
