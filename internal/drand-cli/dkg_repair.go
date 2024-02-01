package drand

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/internal/core"
	"github.com/drand/drand/v2/internal/dkg"

	"github.com/urfave/cli/v2"
)

func NukeDKGStateCmd(c *cli.Context) error {
	baseFolder := c.String(folderFlag.Name)
	beaconID := c.String(beaconIDFlag.Name)
	if !c.IsSet(folderFlag.Name) {
		baseFolder = core.DefaultConfigFolder()
	}
	if !c.IsSet(beaconIDFlag.Name) {
		beaconID = common.DefaultBeaconID
	}

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

	store, err := dkg.NewDKGStore(baseFolder, nil)
	if err != nil {
		return fmt.Errorf("error opening DKG database: %w", err)
	}

	err = store.NukeState(beaconID)
	if err != nil {
		return err
	}
	fmt.Println("DKG state deleted successfully")
	return nil
}
