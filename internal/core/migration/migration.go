package migration

import (
	"path"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/internal/core"
	"github.com/drand/drand/v2/internal/fs"
)

// CheckSBFolderStructure checks if the file structure has been migrated from single-beacon to multi-beacon or not
func CheckSBFolderStructure(baseFolder string) bool {
	groupFolderPath := path.Join(baseFolder, key.GroupFolderName)
	keyFolderPath := path.Join(baseFolder, key.FolderName)
	dbFolderPath := path.Join(baseFolder, core.DefaultDBFolder)
	multiBeaconFolderPath := path.Join(baseFolder, common.MultiBeaconFolder)

	isGroupFound := fs.FolderExists(baseFolder, groupFolderPath)
	isKeyFound := fs.FolderExists(baseFolder, keyFolderPath)
	isDBFound := fs.FolderExists(baseFolder, dbFolderPath)
	isMigrationDone := fs.FolderExists(baseFolder, multiBeaconFolderPath)

	return (isGroupFound || isKeyFound || isDBFound) && !isMigrationDone
}
