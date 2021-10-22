package migration

import (
	"fmt"
	"os"
	"path"

	"github.com/drand/drand/common/constants"

	"github.com/drand/drand/core"
	"github.com/drand/drand/key"

	"github.com/drand/drand/fs"
)

// MigrateOldFolderStructure will migrate the file store structure from drand-single-beacon version
// to the new structure created to support multi-beacon feature. This should be called on any function
// which reads file store from disk, so we are sure the structure is the correct one.
func MigrateOldFolderStructure(baseFolder string) {
	groupFolderPath := path.Join(baseFolder, key.GroupFolderName)
	keyFolderPath := path.Join(baseFolder, key.KeyFolderName)
	dbFolderPath := path.Join(baseFolder, core.DefaultDBFolder)

	isGroupFound := fs.FolderExists(baseFolder, groupFolderPath)
	isKeyFound := fs.FolderExists(baseFolder, keyFolderPath)
	isDBFound := fs.FolderExists(baseFolder, dbFolderPath)

	// Create new folders to move actual files found. If one of them exists, we will be sure the all new structures have been created
	if isGroupFound || isKeyFound || isDBFound {
		if fs.CreateSecureFolder(path.Join(baseFolder, constants.DefaultBeaconID, key.GroupFolderName)) == "" {
			fmt.Println("Something went wrong with the group folder. Make sure that you have the appropriate rights.")
			os.Exit(1)
		}

		if fs.CreateSecureFolder(path.Join(baseFolder, constants.DefaultBeaconID, key.KeyFolderName)) == "" {
			fmt.Println("Something went wrong with the key folder. Make sure that you have the appropriate rights.")
			os.Exit(1)
		}

		if fs.CreateSecureFolder(path.Join(baseFolder, constants.DefaultBeaconID, core.DefaultDBFolder)) == "" {
			fmt.Println("Something went wrong with the db folder. Make sure that you have the appropriate rights.")
			os.Exit(1)
		}
	}

	if isGroupFound {
		oldPath := path.Join(baseFolder, key.GroupFolderName)
		newPath := path.Join(baseFolder, constants.DefaultBeaconID, key.GroupFolderName)

		fmt.Printf("Migrating folder %s to its new path. %s --> %s\n", key.GroupFolderName, oldPath, newPath)

		// Move files to new destinations (only if the folder is found)
		if err := fs.MoveFolder(oldPath, newPath); err != nil {
			fmt.Println("Something went wrong with the new group folder. Make sure that you have the appropriate rights.")
			os.Exit(1)
		}
	}

	if isKeyFound {
		oldPath := path.Join(baseFolder, key.KeyFolderName)
		newPath := path.Join(baseFolder, constants.DefaultBeaconID, key.KeyFolderName)

		fmt.Printf("Migrating folder %s to its new path. %s --> %s\n", key.KeyFolderName, oldPath, newPath)

		// Move files to new destinations (only if the folder is found)
		if err := fs.MoveFolder(oldPath, newPath); err != nil {
			fmt.Println("Something went wrong with the new key folder. Make sure that you have the appropriate rights.")
			os.Exit(1)
		}
	}

	if isDBFound {
		oldPath := path.Join(baseFolder, core.DefaultDBFolder)
		newPath := path.Join(baseFolder, constants.DefaultBeaconID, core.DefaultDBFolder)

		fmt.Printf("Migrating folder %s to its new path. %s --> %s\n", core.DefaultDBFolder, oldPath, newPath)

		// Move files to new destinations (only if the folder is found)
		if err := fs.MoveFolder(oldPath, newPath); err != nil {
			fmt.Println("Something went wrong with the new db folder. Make sure that you have the appropriate rights.")
			os.Exit(1)
		}
	}
}
