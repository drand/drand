package drand

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/internal/core"
	"github.com/drand/drand/v2/internal/dkg"

	"github.com/urfave/cli/v2"
)

// NukeDKGStateCmd is for removing your DKG database and group details for a given beacon.
// the node must be switched off for it to be run.
func NukeDKGStateCmd(c *cli.Context) error {
	baseFolder := c.String(folderFlag.Name)
	beaconID := c.String(beaconIDFlag.Name)
	if !c.IsSet(folderFlag.Name) {
		baseFolder = core.DefaultConfigFolder()
	}
	if !c.IsSet(beaconIDFlag.Name) {
		beaconID = common.DefaultBeaconID
	}

	if err := printPrompt(beaconID, baseFolder); err != nil {
		return err
	}

	store, err := dkg.NewDKGStore(baseFolder, nil)
	if err != nil {
		return fmt.Errorf("error opening DKG database: %w", err)
	}

	if err := store.NukeState(beaconID); err != nil {
		return err
	}

	fileStore := key.NewFileStore(baseFolder, beaconID)
	if err := fileStore.Reset(); err != nil {
		return err
	}

	fmt.Println("DKG state deleted successfully")

	return nil
}

func printPrompt(beaconID, baseFolder string) error {
	fmt.Printf("You are about to nuke the DKG DB state for beacon `%s` located at `%s`.\n", beaconID, baseFolder)
	fmt.Println("For it to be successful, your node should be switched off.")
	fmt.Println()
	fmt.Print("Are you sure you want to proceed???? y/n: ")

	r := bufio.NewReader(os.Stdin)
	s, _ := r.ReadString('\n')
	s = strings.TrimSpace(s)

	if s != "y" {
		return errors.New("aborted by user")
	}
	return nil
}
