// Package fs holds some utilities for manipulating the file system
package fs

import (
	"fmt"
	"os"
	"os/user"
	"path"
)

const defaultDirectoryPermission = 0740
const rwFilePermission = 0600

// HomeFolder returns the home folder of the current user.
func HomeFolder() string {
	u, err := user.Current()
	if err != nil {
		panic(err)
	}
	return u.HomeDir
}

// CreateSecureFolder checks if the folder exists and has the appropriate permission rights. In case of bad permission rights
// the empty string is returned. If the folder doesn't exist it, create it.
func CreateSecureFolder(folder string) string {
	if exists, _ := Exists(folder); !exists {
		if err := os.MkdirAll(folder, defaultDirectoryPermission); err != nil {
			panic(err)
		}
	} else {
		// the folder exists already
		info, err := os.Lstat(folder)
		if err != nil {
			fmt.Println("Error checking stat folder: ", err)
			return ""
		}
		perm := int(info.Mode().Perm())
		if perm != int(defaultDirectoryPermission) {
			fmt.Printf("Folder different permission: %#o vs %#o \n", perm, defaultDirectoryPermission)
			return folder
		}
	}
	return folder
}

// Exists returns whether the given file or directory exists.
func Exists(filePath string) (bool, error) {
	_, err := os.Stat(filePath)
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
	if err := os.Chmod(file, rwFilePermission); err != nil {
		return nil, nil
	}
	return os.OpenFile(file, os.O_RDWR, rwFilePermission)
}

// Files returns the list of file names included in the given path or error if
// any.
func Files(folderPath string) ([]string, error) {
	fi, err := os.ReadDir(folderPath)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, f := range fi {
		if !f.IsDir() {
			files = append(files, path.Join(folderPath, f.Name()))
		}
	}
	return files, nil
}

// Folders returns the list of folder names included in the given path or error if
// any.
func Folders(folderPath string) ([]string, error) {
	fi, err := os.ReadDir(folderPath)
	if err != nil {
		return nil, err
	}
	var folders []string
	for _, f := range fi {
		if f.IsDir() {
			folders = append(folders, path.Join(folderPath, f.Name()))
		}
	}
	return folders, nil
}

// FileExists returns true if the given name is a file in the given path. name
// must be the "basename" of the file and path must be the folder where it lies.
func FileExists(filePath, name string) bool {
	list, err := Files(filePath)
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

// FolderExists returns true if the given name is a folder in the given path. name
// must be the "basename" of the file and path must be the folder where it lies.
func FolderExists(folderPath, name string) bool {
	list, err := Folders(folderPath)
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

// CopyFile copy a file or folder from one path to another
func CopyFile(origFilePath, destFilePath string) error {
	input, err := os.ReadFile(origFilePath)
	if err != nil {
		return err
	}

	err = os.WriteFile(destFilePath, input, rwFilePermission)
	if err != nil {
		return err
	}

	return nil
}

// CopyFolder copy files inside a folder to another folder recursively
func CopyFolder(origFolderPath, destFolderPath string) error {
	fi, err := os.ReadDir(origFolderPath)
	if err != nil {
		return err
	}

	for _, file := range fi {
		tmp1 := path.Join(origFolderPath, file.Name())
		tmp2 := path.Join(destFolderPath, file.Name())

		if !file.IsDir() {
			if err := CopyFile(tmp1, tmp2); err != nil {
				return err
			}
		} else {
			CreateSecureFolder(tmp2)
			if err := CopyFolder(tmp1, tmp2); err != nil {
				return err
			}
		}
	}

	return nil
}
