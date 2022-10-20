package drand

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/drand/drand/common"
	"github.com/drand/drand/core"
	"github.com/drand/drand/core/dkg"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/urfave/cli/v2"
	"github.com/weaveworks/common/fs"
	"google.golang.org/protobuf/types/known/timestamppb"
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
				catchupPeriodFlag,
				proposalFlag,
				secretFlag,
				timeoutFlag,
				dkgTimeoutFlag,
			),
			Action: makeProposal,
		},
		{
			Name: "join",
			Flags: toArray(
				beaconIDFlag,
				controlFlag,
				secretFlag,
				groupFlag,
			),
			Action: joinNetwork,
		},
		{
			Name: "execute",
			Flags: toArray(
				beaconIDFlag,
				controlFlag,
			),
			Action: executeDKG,
		},
		{
			Name: "accept",
			Flags: toArray(
				beaconIDFlag,
				controlFlag,
			),
			Action: acceptDKG,
		},
		{
			Name: "reject",
			Flags: toArray(
				beaconIDFlag,
				controlFlag,
			),
			Action: rejectDKG,
		},
		{
			Name: "abort",
			Flags: toArray(
				beaconIDFlag,
				controlFlag,
			),
			Action: abortDKG,
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

	if isInitialProposal(c) {
		proposal, err := parseInitialProposal(c)
		if err != nil {
			return err
		}

		_, err = client.StartNetwork(context.Background(), proposal)
		if err != nil {
			return err
		}
	} else {
		proposal, err := parseProposal(c)
		if err != nil {
			return err
		}

		_, err = client.StartProposal(context.Background(), proposal)
		if err != nil {
			return err
		}
	}

	fmt.Println("Proposal made successfully!")
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

	// this is IntFlag and not StringFlag so must be checked separately
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

	timeout := time.Now().Add(c.Duration(dkgTimeoutFlag.Name))
	transitionTime := time.Now().Add(c.Duration(transitionTimeFlag.Name))

	return &drand.FirstProposalOptions{
		BeaconID:             beaconID,
		Timeout:              timestamppb.New(timeout),
		Threshold:            uint32(c.Int(thresholdFlag.Name)),
		PeriodSeconds:        uint32(c.Duration(periodFlag.Name).Seconds()),
		Scheme:               c.String(schemeFlag.Name),
		CatchupPeriodSeconds: uint32(c.Duration(catchupPeriodFlag.Name).Seconds()),
		GenesisTime:          timestamppb.New(transitionTime),
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

	proposalFilePath := c.String(proposalFlag.Name)
	proposalFile, err := ParseProposalFile(proposalFilePath)
	if err != nil {
		return nil, err
	}

	if len(proposalFile.Remaining) == 0 {
		return nil, fmt.Errorf("you must provider remainers for a proposal")
	}

	timeout := time.Now().Add(c.Duration(dkgTimeoutFlag.Name))
	transitionTime := time.Now().Add(c.Duration(transitionTimeFlag.Name))

	return &drand.ProposalOptions{
		BeaconID:             beaconID,
		Timeout:              timestamppb.New(timeout),
		TransitionTime:       timestamppb.New(transitionTime),
		Threshold:            uint32(c.Int(thresholdFlag.Name)),
		CatchupPeriodSeconds: uint32(c.Duration(catchupPeriodFlag.Name).Seconds()),
		Joining:              proposalFile.Joining,
		Leaving:              proposalFile.Leaving,
		Remaining:            proposalFile.Remaining,
	}, nil
}

func joinNetwork(c *cli.Context) error {
	beaconID := withDefault(c.String(beaconIDFlag.Name), common.DefaultBeaconID)
	controlPort := withDefault(c.String(controlFlag.Name), core.DefaultControlPort)

	var groupFile []byte
	if c.IsSet(groupFlag.Name) {
		fileContents, err := fs.ReadFile(c.String(groupFlag.Name))
		if err != nil {
			return err
		}
		groupFile = fileContents
	}

	client, err := net.NewDKGControlClient(controlPort)
	if err != nil {
		return err
	}

	_, err = client.StartJoin(context.Background(), &drand.JoinOptions{
		BeaconID:  beaconID,
		GroupFile: groupFile,
	})

	if err == nil {
		fmt.Println("Joined the DKG successfully!")
	}
	return err
}

func executeDKG(c *cli.Context) error {
	err := runSimpleAction(c, func(beaconID string, client drand.DKGControlClient) error {
		_, err := client.StartExecute(context.Background(), &drand.ExecutionOptions{BeaconID: beaconID})
		return err
	})

	if err == nil {
		fmt.Println("DKG execution started successfully!")
	}
	return err
}

func acceptDKG(c *cli.Context) error {
	err := runSimpleAction(c, func(beaconID string, client drand.DKGControlClient) error {
		_, err := client.StartAccept(context.Background(), &drand.AcceptOptions{BeaconID: beaconID})
		return err
	})

	if err == nil {
		fmt.Println("DKG accepted successfully!")
	}
	return err
}

func rejectDKG(c *cli.Context) error {
	err := runSimpleAction(c, func(beaconID string, client drand.DKGControlClient) error {
		_, err := client.StartReject(context.Background(), &drand.RejectOptions{BeaconID: beaconID})
		return err
	})

	if err == nil {
		fmt.Println("DKG rejected successfully!")
	}

	return err
}

func abortDKG(c *cli.Context) error {
	err := runSimpleAction(c, func(beaconID string, client drand.DKGControlClient) error {
		_, err := client.StartAbort(context.Background(), &drand.AbortOptions{BeaconID: beaconID})
		return err
	})
	if err == nil {
		fmt.Println("DKG aborted successfully!")
	}
	return err
}

func runSimpleAction(c *cli.Context, action func(beaconID string, client drand.DKGControlClient) error) error {
	beaconID := withDefault(c.String(beaconIDFlag.Name), common.DefaultBeaconID)
	controlPort := withDefault(c.String(controlFlag.Name), core.DefaultControlPort)

	client, err := net.NewDKGControlClient(controlPort)
	if err != nil {
		return err
	}
	return action(beaconID, client)
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

	if entry.State == uint32(dkg.Fresh) {
		fmt.Println("DKG not yet started!")
		return
	}

	fmt.Printf("BeaconID:\t%s\n", entry.BeaconID)
	fmt.Printf("State:\t\t%s\n", dkg.DKGStatus(entry.State).String())
	fmt.Printf("Epoch:\t\t%d\n", entry.Epoch)
	fmt.Printf("Threshold:\t%d\n", entry.Threshold)
	fmt.Printf("Timeout:\t%s\n", entry.Timeout.AsTime().String())
	fmt.Printf("GenesisTime:\t%s\n", entry.GenesisTime.AsTime().String())
	fmt.Printf("TransitionTime:\t%s\n", entry.TransitionTime.AsTime().String())
	fmt.Printf("GenesisSeed:\t%s\n", hex.EncodeToString(entry.GenesisSeed))
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
			fmt.Printf("\t\t%s\n", member)
		}
		fmt.Println("]")
	}
}

var transitionTimeFlag = &cli.StringFlag{
	Name:  "transition-time",
	Usage: "The duration from now in which keys generated during the next DKG should be used.",
	Value: "30s",
}
var dkgTimeoutFlag = &cli.StringFlag{
	Name:  "timeout",
	Usage: "The duration from now in which DKG participants should abort the DKG if it has not completed.",
	Value: "24h",
}
