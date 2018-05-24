// fs holds some utilities for manipulating the file system
package fs

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"
)

func HomeFolder() string {
	u, err := user.Current()
	if err != nil {
		panic(err)
	}
	return u.HomeDir
}

func CreateHomeConfigFolder(folder string) string {
	u, err := user.Current()
	if err != nil {
		panic(err)
	}
	path := path.Join(u.HomeDir, folder)
	if exists, _ := Exists(path); !exists {
		if err := os.MkdirAll(path, 0740); err != nil {
			panic(err)
		}
	}
	return path
}
func CreateSecureFolder(folder string) string {
	if exists, _ := Exists(folder); !exists {
		if err := os.MkdirAll(folder, 0740); err != nil {
			fmt.Println("folder", folder, ",err", err)
			panic(err)
		}
	}
	return folder
}

// Pwd returns the current directory. Useless for now.
func Pwd() string {
	s, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return s
}

// Exists returns whether the given file or directory exists.
func Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

// CreateSecureFile creates a file with wr permission for user only and returns
// the file handle.
func CreateSecureFile(file string) (*os.File, error) {
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

// Files returns the list of file names included in the given path or error if
// any.
func Files(path string) ([]string, error) {
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

// FileExists returns true if the given name is a file in the given path. name
// must be the "basename" of the file and path must be the folder where it lies.
func FileExists(path string, name string) bool {
	list, err := Files(path)
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
