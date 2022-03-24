package migration

import (
	"fmt"
	"os"
	"path"

	"github.com/drand/drand/common"

	"github.com/drand/drand/core"
	"github.com/drand/drand/key"

	"github.com/drand/drand/fs"
)

// CheckSBFolderStructure checks if the file structure has been migrated from single-beacon to multi-beacon or not
func CheckSBFolderStructure(baseFolder string) bool {
	groupFolderPath := path.Join(baseFolder, key.GroupFolderName)
	keyFolderPath := path.Join(baseFolder, key.KeyFolderName)
	dbFolderPath := path.Join(baseFolder, core.DefaultDBFolder)
	multiBeaconFolderPath := path.Join(baseFolder, common.MultiBeaconFolder)

	isGroupFound := fs.FolderExists(baseFolder, groupFolderPath)
	isKeyFound := fs.FolderExists(baseFolder, keyFolderPath)
	isDBFound := fs.FolderExists(baseFolder, dbFolderPath)
	isMigrationDone := fs.FolderExists(baseFolder, multiBeaconFolderPath)

	return (isGroupFound || isKeyFound || isDBFound) && !isMigrationDone
}

// MigrateSBFolderStructure will migrate the file store structure from single-beacon drand version
// to the new structure created to support multi-beacon feature. This should be called on any function
// which reads file store from disk, so we are sure the structure is the correct one. It will check if
// the process ends successfully or not. If something went wrong, a rollback process will be executed.
func MigrateSBFolderStructure(baseFolder string) error {
	if err := runSBMigration(baseFolder); err != nil {
		multiBeaconFolderPath := path.Join(baseFolder, common.MultiBeaconFolder)
		isMigrationDone := fs.FolderExists(baseFolder, multiBeaconFolderPath)

		if isMigrationDone {
			if localErr := os.RemoveAll(multiBeaconFolderPath); localErr != nil {
				return fmt.Errorf("we could not rollback the migration, please remove %s before run daemon again. Err: %w",
					multiBeaconFolderPath, err)
			}
		}

		return err
	}

	return nil
}

// runSBMigration will migrate the file store structure from single-beacon drand version
// to the new structure created to support multi-beacon feature. It has the logic of the
// migration process.
func runSBMigration(baseFolder string) error {
	groupFolderPath := path.Join(baseFolder, key.GroupFolderName)
	keyFolderPath := path.Join(baseFolder, key.KeyFolderName)
	dbFolderPath := path.Join(baseFolder, core.DefaultDBFolder)
	multiBeaconFolderPath := path.Join(baseFolder, common.MultiBeaconFolder)

	isGroupFound := fs.FolderExists(baseFolder, groupFolderPath)
	isKeyFound := fs.FolderExists(baseFolder, keyFolderPath)
	isDBFound := fs.FolderExists(baseFolder, dbFolderPath)
	isMigrationDone := fs.FolderExists(baseFolder, multiBeaconFolderPath)

	if isMigrationDone {
		return nil
	}

	if fs.CreateSecureFolder(path.Join(baseFolder, common.MultiBeaconFolder)) == "" {
		return fmt.Errorf("something went wrong with the multi beacon folder, make sure that you have the appropriate rights")
	}

	// Create new folders to move actual files found. If one of them exists, we will be sure the all new structures have been created
	if isGroupFound || isKeyFound || isDBFound {
		if fs.CreateSecureFolder(path.Join(baseFolder, common.MultiBeaconFolder, common.DefaultBeaconID, key.GroupFolderName)) == "" {
			return fmt.Errorf("something went wrong with the group folder, make sure that you have the appropriate rights")
		}

		if fs.CreateSecureFolder(path.Join(baseFolder, common.MultiBeaconFolder, common.DefaultBeaconID, key.KeyFolderName)) == "" {
			return fmt.Errorf("something went wrong with the key folder, make sure that you have the appropriate rights")
		}

		if fs.CreateSecureFolder(path.Join(baseFolder, common.MultiBeaconFolder, common.DefaultBeaconID, core.DefaultDBFolder)) == "" {
			return fmt.Errorf("something went wrong with the db folder, make sure that you have the appropriate rights")
		}
	}

	if isGroupFound {
		oldPath := path.Join(baseFolder, key.GroupFolderName)
		newPath := path.Join(baseFolder, common.MultiBeaconFolder, common.DefaultBeaconID, key.GroupFolderName)

		// Copy files to new destinations (only if the folder is found)
		if err := fs.CopyFolder(oldPath, newPath); err != nil {
			return fmt.Errorf("something went wrong with the new group folder. make sure that you have the appropriate rights")
		}
	}

	if isKeyFound {
		oldPath := path.Join(baseFolder, key.KeyFolderName)
		newPath := path.Join(baseFolder, common.MultiBeaconFolder, common.DefaultBeaconID, key.KeyFolderName)

		// Copy files to new destinations (only if the folder is found)
		if err := fs.CopyFolder(oldPath, newPath); err != nil {
			return fmt.Errorf("something went wrong with the new key folder. make sure that you have the appropriate rights")
		}
	}

	if isDBFound {
		oldPath := path.Join(baseFolder, core.DefaultDBFolder)
		newPath := path.Join(baseFolder, common.MultiBeaconFolder, common.DefaultBeaconID, core.DefaultDBFolder)

		// Copy files to new destinations (only if the folder is found)
		if err := fs.CopyFolder(oldPath, newPath); err != nil {
			return fmt.Errorf("something went wrong with the new db folder. make sure that you have the appropriate rights")
		}
	}

	return nil
}
