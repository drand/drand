package drand

import (
	"context"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/drand/drand/common"
	"github.com/drand/drand/core"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

var dkgCommand = &cli.Command{
	Name:  "dkg",
	Usage: "Commands for interacting with the DKG",
	Subcommands: []*cli.Command{
		{
			Name: "propose",
			Flags: toArray(
				beaconIDFlag,
				controlFlag,
				schemeFlag,
				periodFlag,
				thresholdFlag,
				timeoutFlag,
				catchupPeriodFlag,
				proposalFlag,
				secretFlag,
			),
			Action: makeProposal,
		},
		{
			Name: "status",
			Flags: toArray(
				beaconIDFlag,
				controlFlag,
			),
			Action: viewStatus,
		},
	},
}

func makeProposal(c *cli.Context) error {
	controlPort := withDefault(c.String(controlFlag.Name), core.DefaultControlPort)
	client, err := net.NewDKGControlClient(controlPort)
	if err != nil {
		return err
	}

	var proposalResponse *drand.GenericResponseMessage
	if isInitialProposal(c) {
		proposal, err := parseInitialDKGProposal(c)
		if err != nil {
			return err
		}

		proposalResponse, err = client.StartNetwork(context.Background(), proposal)
		if err != nil {
			return err
		}

	} else {
		proposal, err := parseDKGProposal(c)
		if err != nil {
			return err
		}

		proposalResponse, err = client.StartProposal(context.Background(), proposal)
		if err != nil {
			return err
		}
	}

	if proposalResponse.IsError {
		return fmt.Errorf("error: %s, code: %s", proposalResponse.ErrorMessage, core.DKGErrorCode(proposalResponse.ErrorCode).String())
	}

	fmt.Println("proposal made successfully!")
	return nil
}

func isInitialProposal(c *cli.Context) bool {
	return c.IsSet(schemeFlag.Name)
}

func withDefault(first, second string) string {
	if first == "" {
		return second
	}

	return first
}

func parseInitialDKGProposal(c *cli.Context) (*drand.FirstProposalOptions, error) {
	beaconID := withDefault(c.String(beaconIDFlag.Name), common.DefaultBeaconID)
	requiredFlags := []*cli.StringFlag{proposalFlag, periodFlag, schemeFlag, catchupPeriodFlag}

	for _, flag := range requiredFlags {
		if !c.IsSet(flag.Name) {
			return nil, fmt.Errorf("%s flag is required for initial proposals", flag.Name)
		}
	}

	if !c.IsSet(thresholdFlag.Name) {
		return nil, fmt.Errorf("%s flag is required for initial proposals", thresholdFlag.Name)
	}

	proposalFile := ProposalFile{}
	_, err := toml.DecodeFile(c.String(proposalFlag.Name), &proposalFile)

	if err != nil {
		return nil, err
	}

	if proposalFile.Leavers() != nil || proposalFile.Remainers() != nil {
		return nil, fmt.Errorf("your proposal file must not have `Leaving` or `Remaining` for an initial DKG proposal")
	}

	if proposalFile.Joiners() == nil {
		return nil, fmt.Errorf("your proposal file must have `Joining`")
	}

	return &drand.FirstProposalOptions{
		BeaconID:             beaconID,
		Timeout:              timestamppb.New(time.Now().Add(24 * time.Hour)),
		Threshold:            uint32(c.Int(thresholdFlag.Name)),
		PeriodSeconds:        uint32(c.Duration(periodFlag.Name).Seconds()),
		Scheme:               c.String(schemeFlag.Name),
		CatchupPeriodSeconds: uint32(c.Duration(catchupPeriodFlag.Name).Seconds()),
		GenesisTime:          nil,
		GenesisSeed:          []byte(""),
		Joining:              proposalFile.Joiners(),
	}, nil
}

func parseDKGProposal(c *cli.Context) (*drand.ProposalOptions, error) {
	beaconID := withDefault(c.String(beaconIDFlag.Name), common.DefaultBeaconID)
	bannedFlags := []*cli.StringFlag{periodFlag, schemeFlag}
	for _, flag := range bannedFlags {
		if c.IsSet(flag.Name) {
			return nil, fmt.Errorf("%s flag can only be set for initial proposals", flag.Name)
		}
	}

	if !c.IsSet(proposalFlag.Name) {
		return nil, fmt.Errorf("%s flag is required ", proposalFlag.Name)
	}

	if !c.IsSet(thresholdFlag.Name) {
		return nil, fmt.Errorf("%s flag is required", thresholdFlag.Name)
	}

	proposalFile := ProposalFile{}
	_, err := toml.DecodeFile(c.String(proposalFlag.Name), &proposalFile)

	if err != nil {
		return nil, err
	}

	if proposalFile.Remainers() == nil {
		return nil, fmt.Errorf("you must provider remainers for a proposal")
	}

	return &drand.ProposalOptions{
		BeaconID:             beaconID,
		Timeout:              timestamppb.New(time.Now().Add(24 * time.Hour)),
		Threshold:            uint32(c.Int(thresholdFlag.Name)),
		CatchupPeriodSeconds: uint32(c.Duration(catchupPeriodFlag.Name).Seconds()),
		Joining:              proposalFile.Joiners(),
		Leaving:              proposalFile.Leavers(),
		Remaining:            proposalFile.Remainers(),
	}, nil
}

func viewStatus(c *cli.Context) error {
	var beaconID string
	if c.IsSet(beaconIDFlag.Name) {
		beaconID = c.String(beaconIDFlag.Name)
	} else {
		beaconID = common.DefaultBeaconID
	}

	var controlPort string
	if c.IsSet(controlFlag.Name) {
		controlPort = c.String(controlFlag.Name)
	} else {
		controlPort = core.DefaultControlPort
	}

	client, err := net.NewDKGControlClient(controlPort)
	if err != nil {
		return err
	}

	status, err := client.DKGStatus(context.Background(), &drand.DKGStatusRequest{BeaconID: beaconID})
	if err != nil {
		return err
	}

	fmt.Println("CURRENT")
	fmt.Println(status.Current)
	fmt.Println("PREVIOUS")
	fmt.Println(status.Complete)
	return nil
}
