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

func MigrateOldFolderStructure(baseFolder string) {
	groupFolderPath := path.Join(baseFolder, key.GroupFolderName)
	keyFolderPath := path.Join(baseFolder, key.KeyFolderName)
	dbFolderPath := path.Join(baseFolder, core.DefaultDBFolder)

	isGroupFound := fs.FileExists(baseFolder, groupFolderPath)
	isKeyFound := fs.FileExists(baseFolder, keyFolderPath)
	isDBFound := fs.FileExists(baseFolder, dbFolderPath)

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
		if err := fs.MoveFolder(path.Join(baseFolder, key.GroupFolderName),
			path.Join(baseFolder, constants.DefaultBeaconID, key.GroupFolderName)); err != nil {
			fmt.Println("Something went wrong with the new group folder. Make sure that you have the appropriate rights.")
			os.Exit(1)
		}
	}

	if isKeyFound {
		// Move files to new destinations (only if the folder is found)
		if err := fs.MoveFolder(path.Join(baseFolder, key.KeyFolderName),
			path.Join(baseFolder, constants.DefaultBeaconID, key.KeyFolderName)); err != nil {
			fmt.Println("Something went wrong with the new key folder. Make sure that you have the appropriate rights.")
			os.Exit(1)
		}
	}

	if isDBFound {
		if err := fs.MoveFolder(path.Join(baseFolder, core.DefaultDBFolder),
			path.Join(baseFolder, constants.DefaultBeaconID, core.DefaultDBFolder)); err != nil {
			fmt.Println("Something went wrong with the new db folder. Make sure that you have the appropriate rights.")
			os.Exit(1)
		}
	}
}
