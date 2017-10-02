package main

import (
	"io/ioutil"
	"os"
)

func createSecureFile(file string) (*os.File, error) {
	fd, err := os.Create(file)
	if err != nil {
		return nil, err
	}
	fd.Close()
	if err := os.Chmod(file, 0600); err != nil {
		return nil, nil
	}
	return os.OpenFile(file, os.O_RDWR, 0600)
}

// files returns the list of file names included in the given path or error if
// any.
func files(path string) ([]string, error) {
	fi, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, f := range fi {
		if !f.IsDir() {
			files = append(files, f.Name())
		}
	}
	return files, nil
}

// exists returns true if the given name is a file in the given path. name must
// be the "basename" of the file.
func fileExists(path string, name string) bool {
	list, err := files(path)
	if err != nil {
		return false
	}

	for _, l := range list {
		if l == name {
			return true
		}
	}

	return false
}
