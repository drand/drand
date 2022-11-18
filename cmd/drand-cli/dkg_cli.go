package drand

import (
	"context"
	"fmt"
	"github.com/drand/drand/common"
	"github.com/drand/drand/core"
	"github.com/drand/drand/core/dkg"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log"
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
			Name: "join",
			Flags: toArray(
				beaconIDFlag,
				controlFlag,
				secretFlag,
			),
			Action: joinNetwork,
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
		proposal, err := parseInitialProposal(c)
		if err != nil {
			return err
		}

		proposalResponse, err = client.StartNetwork(context.Background(), proposal)
		if err != nil {
			return err
		}

	} else {
		proposal, err := parseProposal(c)
		if err != nil {
			return err
		}

		proposalResponse, err = client.StartProposal(context.Background(), proposal)
		if err != nil {
			return err
		}
	}

	if proposalResponse.IsError {
		return fmt.Errorf("error making proposal: %s", proposalResponse.ErrorMessage)
	}

	log.Default().Println("proposal made successfully!")
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

func parseInitialProposal(c *cli.Context) (*drand.FirstProposalOptions, error) {
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

	proposalFile, err := ParseProposalFile(c.String(proposalFlag.Name))
	if err != nil {
		return nil, err
	}

	err = validateInitialProposal(proposalFile)
	if err != nil {
		return nil, err
	}

	return &drand.FirstProposalOptions{
		BeaconID:             beaconID,
		Timeout:              timestamppb.New(time.Now().Add(24 * time.Hour)),
		Threshold:            uint32(c.Int(thresholdFlag.Name)),
		PeriodSeconds:        uint32(c.Duration(periodFlag.Name).Seconds()),
		Scheme:               c.String(schemeFlag.Name),
		CatchupPeriodSeconds: uint32(c.Duration(catchupPeriodFlag.Name).Seconds()),
		GenesisTime:          timestamppb.New(time.Now().Add(1 * time.Minute)),
		GenesisSeed:          []byte(""),
		Joining:              proposalFile.Joining,
	}, nil
}

func validateInitialProposal(proposalFile *ProposalFile) error {
	if len(proposalFile.Leaving) != 0 || len(proposalFile.Remaining) != 0 {
		return fmt.Errorf("your proposal file must not have `Leaving` or `Remaining` for an initial DKG proposal")
	}

	if len(proposalFile.Joining) == 0 {
		return fmt.Errorf("your proposal file must have `Joining`")
	}

	return nil
}

func parseProposal(c *cli.Context) (*drand.ProposalOptions, error) {
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

	proposalFile, err := ParseProposalFile(c.String(proposalFlag.Name))
	if err != nil {
		return nil, err
	}

	if len(proposalFile.Remaining) == 0 {
		return nil, fmt.Errorf("you must provider remainers for a proposal")
	}

	return &drand.ProposalOptions{
		BeaconID:             beaconID,
		Timeout:              timestamppb.New(time.Now().Add(24 * time.Hour)),
		Threshold:            uint32(c.Int(thresholdFlag.Name)),
		CatchupPeriodSeconds: uint32(c.Duration(catchupPeriodFlag.Name).Seconds()),
		Joining:              proposalFile.Joining,
		Leaving:              proposalFile.Leaving,
		Remaining:            proposalFile.Remaining,
	}, nil
}

func joinNetwork(c *cli.Context) error {
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

	response, err := client.StartJoin(context.Background(), &drand.JoinOptions{BeaconID: beaconID})

	if err != nil {
		return err
	}

	if response.IsError {
		return fmt.Errorf("error joining network: %s", response.ErrorMessage)
	}

	log.Default().Println("Joined the DKG successfully!")
	return nil
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

	fmt.Println("<<Current>>")
	printEntry(status.Current)
	fmt.Println()
	fmt.Println("<<Last Completed>>")
	printEntry(status.Complete)
	return nil
}

func printEntry(entry *drand.DKGEntry) {
	if entry == nil {
		fmt.Println("DKG entry nil")
		return
	}

	fmt.Printf("BeaconID:\t\t%s\n", entry.BeaconID)
	fmt.Printf("State:\t\t%s\n", dkg.DKGStatus(entry.State).String())
	fmt.Printf("Epoch:\t\t%d\n", entry.Epoch)
	fmt.Printf("Threshold:\t%d\n", entry.Threshold)
	fmt.Printf("Timeout:\t%s\n", entry.Timeout.AsTime().String())
	fmt.Printf("Leader:\t\t%s\n", entry.Leader.Address)
	fmt.Println("Joining: [")
	for _, joiner := range entry.Joining {
		fmt.Printf("\t\t%s\n", joiner.Address)
	}
	fmt.Println("]")

	fmt.Println("Leaving: [")
	for _, leaver := range entry.Leaving {
		fmt.Printf("\t\t%s\n", leaver.Address)
	}
	fmt.Println("]")

	fmt.Println("Remaining: [")
	for _, remainer := range entry.Remaining {
		fmt.Printf("\t\t%s\n", remainer.Address)
	}
	fmt.Println("]")

	if dkg.DKGStatus(entry.State) == dkg.Proposing {
		fmt.Println("Accepted: [")
		for _, acceptor := range entry.Acceptors {
			fmt.Printf("\t\t%s\n", acceptor.Address)
		}
		fmt.Println("]")

		fmt.Println("Rejected: [")
		for _, rejector := range entry.Rejectors {
			fmt.Printf("\t\t%s\n", rejector.Address)
		}
		fmt.Println("]")
	}

	if dkg.DKGStatus(entry.State) >= dkg.Executing {
		fmt.Println("Final group: [")
		for _, member := range entry.FinalGroup {
			fmt.Printf("\t\t%s\n", member.Address)
		}
		fmt.Println("]")
	}
}
