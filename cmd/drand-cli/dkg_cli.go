package drand

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/dkg"

	"github.com/drand/drand/common"
	"github.com/drand/drand/core"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/urfave/cli/v2"
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
				dkgTimeoutFlag,
				transitionTimeFlag,
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
				formatFlag,
			),
			Action: viewStatus,
		},
	},
}

var formatFlag = &cli.StringFlag{
	Name:    "format",
	Usage:   "Set the format of the status output. Valid options are: pretty, csv",
	Value:   "pretty",
	EnvVars: []string{"DRAND_STATUS_FORMAT"},
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

	if c.IsSet(transitionTimeFlag.Name) {
		return nil, fmt.Errorf("%s flag may not be set for initial proposals", transitionTimeFlag.Name)
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

	// parse a proposal file from the path specified
	proposalFilePath := c.String(proposalFlag.Name)
	proposalFile, err := ParseProposalFile(proposalFilePath)
	if err != nil {
		return nil, err
	}

	if len(proposalFile.Remaining) == 0 {
		return nil, fmt.Errorf("you must provider remainers for a proposal")
	}

	// figure out the round closest to the transition duration provided

	timeout := time.Now().Add(c.Duration(dkgTimeoutFlag.Name))
	var transitionTime time.Time
	if c.IsSet(transitionTimeFlag.Name) {
		transitionTime = time.Now().Add(c.Duration(transitionTimeFlag.Name))
	} else {
		transitionTime = time.Now().Add(1 * time.Minute)
	}

	// first we get the chainInfo for the beacon
	var ctrlPort string
	if c.IsSet(controlFlag.Name) {
		ctrlPort = c.String(controlFlag.Name)
	} else {
		ctrlPort = core.DefaultControlPort
	}

	ctrlClient, err := net.NewControlClient(ctrlPort)
	info, err := ctrlClient.ChainInfo(beaconID)

	// then we use it to work out the real transition time
	transitionRound := chain.CurrentRound(transitionTime.Unix(), time.Duration(info.Period)*time.Second, info.GenesisTime)
	actualTransitionTime := chain.TimeOfRound(time.Duration(info.Period)*time.Second, info.GenesisTime, transitionRound)

	return &drand.ProposalOptions{
		BeaconID:             beaconID,
		Timeout:              timestamppb.New(timeout),
		TransitionTime:       timestamppb.New(time.Unix(actualTransitionTime, 0)),
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
		fileContents, err := os.ReadFile(c.String(groupFlag.Name))
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

	if !c.IsSet(formatFlag.Name) || c.String(formatFlag.Name) == "pretty" {
		prettyPrint(c, "<<Current>>", status.Current)
		prettyPrint(c, "<<Completed>>", status.Complete)
	} else if c.String(formatFlag.Name) == "csv" {
		csvPrint(c, "<<Current>>", status.Current)
		csvPrint(c, "<<Completed>>", status.Complete)
	} else {
		return errors.New("invalid format flag")
	}
	return nil
}

func csvPrint(c *cli.Context, tag string, entry *drand.DKGEntry) {
	out := c.App.Writer
	_, _ = fmt.Fprintf(out, "%s", tag)
	if entry == nil {
		_, _ = fmt.Fprintln(out, "nil")
		return
	}

	_, err := fmt.Fprintf(
		c.App.Writer,
		"BeaconID:%s,State:%s,Epoch:%d,Threshold:%d,Timeout:%s,GenesisTime:%s,TransitionTime:%s,GenesisSeed:%s,Leader:%s",
		entry.BeaconID,
		dkg.DKGStatus(entry.State).String(),
		entry.Epoch,
		entry.Threshold,
		entry.Timeout.AsTime().String(),
		entry.GenesisTime.AsTime().String(),
		entry.TransitionTime.AsTime().String(),
		hex.EncodeToString(entry.GenesisSeed),
		entry.Leader.Address,
	)
	if err != nil {
		_, _ = fmt.Fprintln(out, "error")
	}
}

func prettyPrint(c *cli.Context, tag string, entry *drand.DKGEntry) {
	out := c.App.Writer

	_, _ = fmt.Fprintln(out, tag)
	if entry == nil {
		_, _ = fmt.Fprintln(out, "nil")
		return
	}

	if entry.State == uint32(dkg.Fresh) {
		_, _ = fmt.Fprintln(out, "DKG not yet started!")
		return
	}

	_, _ = fmt.Fprintf(out, "BeaconID:\t%s\n", entry.BeaconID)
	_, _ = fmt.Fprintf(out, "State:\t\t%s\n", dkg.DKGStatus(entry.State).String())
	_, _ = fmt.Fprintf(out, "Epoch:\t\t%d\n", entry.Epoch)
	_, _ = fmt.Fprintf(out, "Threshold:\t%d\n", entry.Threshold)
	_, _ = fmt.Fprintf(out, "Timeout:\t%s\n", entry.Timeout.AsTime().String())
	_, _ = fmt.Fprintf(out, "GenesisTime:\t%s\n", entry.GenesisTime.AsTime().String())
	_, _ = fmt.Fprintf(out, "TransitionTime:\t%s\n", entry.TransitionTime.AsTime().String())
	_, _ = fmt.Fprintf(out, "GenesisSeed:\t%s\n", hex.EncodeToString(entry.GenesisSeed))
	_, _ = fmt.Fprintf(out, "Leader:\t\t%s\n", entry.Leader.Address)
	_, _ = fmt.Fprint(out, "Joining: [")
	for _, joiner := range entry.Joining {
		_, _ = fmt.Fprintf(out, "\t\t%s\n", joiner.Address)
	}
	_, _ = fmt.Fprintln(out, "]")

	_, _ = fmt.Fprintln(out, "Leaving: [")
	for _, leaver := range entry.Leaving {
		_, _ = fmt.Fprintf(out, "\t\t%s\n", leaver.Address)
	}
	_, _ = fmt.Fprintln(out, "]")

	_, _ = fmt.Fprintln(out, "Remaining: [")
	for _, remainer := range entry.Remaining {
		_, _ = fmt.Fprintf(out, "\t\t%s\n", remainer.Address)
	}
	_, _ = fmt.Fprintln(out, "]")

	if dkg.DKGStatus(entry.State) == dkg.Proposing {
		_, _ = fmt.Fprintln(out, "Accepted: [")
		for _, acceptor := range entry.Acceptors {
			_, _ = fmt.Fprintf(out, "\t\t%s\n", acceptor.Address)
		}
		_, _ = fmt.Fprintln(out, "]")

		_, _ = fmt.Fprintln(out, "Rejected: [")
		for _, rejector := range entry.Rejectors {
			_, _ = fmt.Fprintf(out, "\t\t%s\n", rejector.Address)
		}
		_, _ = fmt.Fprintln(out, "]")
	}

	if dkg.DKGStatus(entry.State) >= dkg.Executing {
		_, _ = fmt.Fprintln(out, "Final group: [")
		for _, member := range entry.FinalGroup {
			_, _ = fmt.Fprintf(out, "\t\t%s\n", member)
		}
		_, _ = fmt.Fprintln(out, "]")
	}
}

var transitionTimeFlag = &cli.StringFlag{
	Name:  "transition-time",
	Usage: "The duration from now until which keys generated during the next DKG should be used. It will be modified to the nearest round.",
	Value: "30s",
}
var dkgTimeoutFlag = &cli.StringFlag{
	Name:  "timeout",
	Usage: "The duration from now in which DKG participants should abort the DKG if it has not completed.",
	Value: "24h",
}
