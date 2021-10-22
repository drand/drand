package migration

import (
	"fmt"
	"path"

	"github.com/drand/drand/common/constants"

	"github.com/drand/drand/core"
	"github.com/drand/drand/key"

	"github.com/drand/drand/fs"
)

// MigrateOldFolderStructure will migrate the file store structure from drand-single-beacon version
// to the new structure created to support multi-beacon feature. This should be called on any function
// which reads file store from disk, so we are sure the structure is the correct one.
func MigrateOldFolderStructure(baseFolder string) error {
	groupFolderPath := path.Join(baseFolder, key.GroupFolderName)
	keyFolderPath := path.Join(baseFolder, key.KeyFolderName)
	dbFolderPath := path.Join(baseFolder, core.DefaultDBFolder)
	defaultBeaconPath := path.Join(baseFolder, constants.DefaultBeaconID)

	isGroupFound := fs.FolderExists(baseFolder, groupFolderPath)
	isKeyFound := fs.FolderExists(baseFolder, keyFolderPath)
	isDBFound := fs.FolderExists(baseFolder, dbFolderPath)
	isDefaultBeaconFound := fs.FolderExists(baseFolder, defaultBeaconPath)

	// Create new folders to move actual files found. If one of them exists, we will be sure the all new structures have been created
	if isGroupFound || isKeyFound || isDBFound {
		if isDefaultBeaconFound {
			return fmt.Errorf("default beacon folder already exists. Cannot move files into it. Remove it first")
		}

		if fs.CreateSecureFolder(path.Join(baseFolder, constants.DefaultBeaconID, key.GroupFolderName)) == "" {
			return fmt.Errorf("something went wrong with the group folder. Make sure that you have the appropriate rights")
		}

		if fs.CreateSecureFolder(path.Join(baseFolder, constants.DefaultBeaconID, key.KeyFolderName)) == "" {
			return fmt.Errorf("something went wrong with the key folder. Make sure that you have the appropriate rights")
		}

		if fs.CreateSecureFolder(path.Join(baseFolder, constants.DefaultBeaconID, core.DefaultDBFolder)) == "" {
			return fmt.Errorf("something went wrong with the db folder. Make sure that you have the appropriate rights")
		}
	}

	if isGroupFound {
		oldPath := path.Join(baseFolder, key.GroupFolderName)
		newPath := path.Join(baseFolder, constants.DefaultBeaconID, key.GroupFolderName)

		// Move files to new destinations (only if the folder is found)
		if err := fs.MoveFolder(oldPath, newPath); err != nil {
			return fmt.Errorf("something went wrong with the new group folder. Make sure that you have the appropriate rights")
		}
	}

	if isKeyFound {
		oldPath := path.Join(baseFolder, key.KeyFolderName)
		newPath := path.Join(baseFolder, constants.DefaultBeaconID, key.KeyFolderName)

		// Move files to new destinations (only if the folder is found)
		if err := fs.MoveFolder(oldPath, newPath); err != nil {
			return fmt.Errorf("something went wrong with the new key folder. Make sure that you have the appropriate rights")
		}
	}

	if isDBFound {
		oldPath := path.Join(baseFolder, core.DefaultDBFolder)
		newPath := path.Join(baseFolder, constants.DefaultBeaconID, core.DefaultDBFolder)

		// Move files to new destinations (only if the folder is found)
		if err := fs.MoveFolder(oldPath, newPath); err != nil {
			return fmt.Errorf("something went wrong with the new db folder. Make sure that you have the appropriate rights")
		}
	}

	return nil
}
