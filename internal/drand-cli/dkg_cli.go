package drand

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/drand/drand/common"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/chain"
	"github.com/drand/drand/internal/core"
	"github.com/drand/drand/internal/dkg"
	"github.com/drand/drand/internal/net"
	common2 "github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
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
			Action: func(c *cli.Context) error {
				l := log.New(nil, logLevel(c), logJSON(c)).
					Named("dkgPropose")
				return makeProposal(c, l)
			},
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
		{
			Name: "generate-proposal",
			Flags: toArray(
				joinerFlag,
				remainerFlag,
				proposalOutputFlag,
				beaconIDFlag,
				leaverFlag,
			),
			Action: func(c *cli.Context) error {
				l := log.New(nil, logLevel(c), logJSON(c)).
					Named("dkgGenerateProposal")
				return generateProposalCmd(c, l)
			},
		},
		{
			Name: "migrate",
			Flags: toArray(
				controlFlag,
			),
			Action: migrateDKG,
		},
	},
}

func migrateDKG(c *cli.Context) error {
	if !c.IsSet(controlFlag.Name) {
		return errors.New("you must set the control port")
	}

	port := c.String(controlFlag.Name)
	ctrl, err := net.NewControlClient(log.DefaultLogger(), port)
	if err != nil {
		return err
	}

	err = ctrl.Migrate()
	if err == nil {
		fmt.Println("Migration completed successfully!")
	}
	return err
}

var joinerFlag = &cli.StringSliceFlag{
	Name: "joiner",
	Usage: "the address of a joiner you wish to add to a DKG proposal. You can pass it multiple times. " +
		"To use TLS, prefix their address with 'https://'",
}

var remainerFlag = &cli.StringSliceFlag{
	Name: "remainer",
	Usage: "the address of a remainer you wish to add to a DKG proposal. You can pass it multiple times. " +
		"To use TLS, prefix their address with 'https://'",
}

var leaverFlag = &cli.StringSliceFlag{
	Name: "leaver",
	Usage: "the address of a leaver you wish to add to the DKG proposal. You can pass it multiple times. " +
		"To use TLS, prefix their address with 'https://'",
}

var proposalOutputFlag = &cli.StringFlag{
	Name:  "out",
	Usage: "the location you wish to save the proposal file to",
}

var formatFlag = &cli.StringFlag{
	Name:    "format",
	Usage:   "Set the format of the status output. Valid options are: pretty, csv",
	Value:   "pretty",
	EnvVars: []string{"DRAND_STATUS_FORMAT"},
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

func makeProposal(c *cli.Context, l log.Logger) error {
	controlPort := withDefault(c.String(controlFlag.Name), core.DefaultControlPort)
	client, err := net.NewDKGControlClient(l, controlPort)
	if err != nil {
		return err
	}

	if isInitialProposal(c) {
		proposal, err := parseInitialProposal(c)
		if err != nil {
			return err
		}

		_, err = client.StartNetwork(c.Context, proposal)
		if err != nil {
			return err
		}
	} else {
		proposal, err := parseProposal(c, l)
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

	period := c.Duration(periodFlag.Name)
	timeout := time.Now().Add(c.Duration(dkgTimeoutFlag.Name))

	// if a custom time hasn't been added, we set a one based on the period
	// to make the tests a little more predictable
	var genesisTime time.Time
	if c.IsSet(transitionTimeFlag.Name) {
		genesisTime = time.Now().Add(c.Duration(transitionTimeFlag.Name))
	} else {
		defaultRoundsToWait := period * 3
		genesisTime = time.Now().Add(defaultRoundsToWait)
	}

	return &drand.FirstProposalOptions{
		BeaconID:             beaconID,
		Timeout:              timestamppb.New(timeout),
		Threshold:            uint32(c.Int(thresholdFlag.Name)),
		PeriodSeconds:        uint32(period.Seconds()),
		Scheme:               c.String(schemeFlag.Name),
		CatchupPeriodSeconds: uint32(c.Duration(catchupPeriodFlag.Name).Seconds()),
		GenesisTime:          timestamppb.New(genesisTime),
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

func parseProposal(c *cli.Context, l log.Logger) (*drand.ProposalOptions, error) {
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

	var timeout time.Time
	if c.IsSet(dkgTimeoutFlag.Name) {
		timeout = time.Now().Add(c.Duration(dkgTimeoutFlag.Name))
	} else {
		timeout = time.Now().Add(core.DefaultDKGTimeout)
	}

	// figure out the round closest to the transition duration provided
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

	ctrlClient, err := net.NewControlClient(l, ctrlPort)
	if err != nil {
		return nil, err
	}
	info, err := ctrlClient.ChainInfo(beaconID)
	if err != nil {
		return nil, err
	}

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
	l := log.FromContextOrDefault(c.Context)
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

	client, err := net.NewDKGControlClient(l, controlPort)
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
	l := log.FromContextOrDefault(c.Context)
	beaconID := withDefault(c.String(beaconIDFlag.Name), common.DefaultBeaconID)
	controlPort := withDefault(c.String(controlFlag.Name), core.DefaultControlPort)

	client, err := net.NewDKGControlClient(l, controlPort)
	if err != nil {
		return err
	}
	return action(beaconID, client)
}

func viewStatus(c *cli.Context) error {
	l := log.FromContextOrDefault(c.Context)
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

	client, err := net.NewDKGControlClient(l, controlPort)
	if err != nil {
		return err
	}

	status, err := client.DKGStatus(context.Background(), &drand.DKGStatusRequest{BeaconID: beaconID})
	if err != nil {
		return err
	}

	if !c.IsSet(formatFlag.Name) || c.String(formatFlag.Name) == "pretty" {
		prettyPrint(status)
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
		dkg.Status(entry.State).String(),
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

func prettyPrint(status *drand.DKGStatusResponse) {
	tw := table.NewWriter()
	tw.AppendHeader(table.Row{"Field", "Current", "Finished"})

	if dkg.Status(status.Current.State) == dkg.Fresh {
		tw.AppendRow(table.Row{"State", "Fresh", "Fresh"})
		fmt.Println(tw.Render())
		return
	}

	orEmpty := func(entry *drand.DKGEntry, mapping func(entry *drand.DKGEntry) any) any {
		if entry == nil {
			return ""
		}
		return mapping(entry)
	}

	appendRow := func(key string, status *drand.DKGStatusResponse, mapping func(entry *drand.DKGEntry) any) {
		tw.AppendRow(table.Row{key, orEmpty(status.Current, mapping), orEmpty(status.Complete, mapping)})
	}

	formatAddresses := func(arr []*drand.Participant) string {
		if len(arr) == 0 {
			return "[]"
		}

		b := strings.Builder{}
		b.WriteString("[")
		for _, a := range arr {
			b.WriteString(fmt.Sprintf("\n\t%s,", a.Address))
		}
		b.WriteString("\n]")

		return b.String()
	}

	appendRow("Status", status, func(entry *drand.DKGEntry) any {
		return dkg.Status(entry.State).String()
	})
	appendRow("BeaconID", status, func(entry *drand.DKGEntry) any {
		return entry.BeaconID
	})
	appendRow("Epoch", status, func(entry *drand.DKGEntry) any {
		return entry.Epoch
	})
	appendRow("Threshold", status, func(entry *drand.DKGEntry) any {
		return entry.Threshold
	})
	appendRow("Timeout", status, func(entry *drand.DKGEntry) any {
		return entry.Timeout.AsTime().String()
	})
	appendRow("GenesisTime", status, func(entry *drand.DKGEntry) any {
		return entry.GenesisTime.AsTime().String()
	})
	appendRow("TransitionTime", status, func(entry *drand.DKGEntry) any {
		return entry.TransitionTime.AsTime().String()
	})
	appendRow("GenesisSeed", status, func(entry *drand.DKGEntry) any {
		return hex.EncodeToString(entry.GenesisSeed)
	})
	appendRow("Leader", status, func(entry *drand.DKGEntry) any {
		return entry.Leader.Address
	})
	appendRow("Joining", status, func(entry *drand.DKGEntry) any {
		return formatAddresses(entry.Joining)
	})
	appendRow("Remaining", status, func(entry *drand.DKGEntry) any {
		return formatAddresses(entry.Remaining)
	})
	appendRow("Leaving", status, func(entry *drand.DKGEntry) any {
		return formatAddresses(entry.Leaving)
	})

	if dkg.Status(status.Current.State) == dkg.Proposing {
		appendRow("Accepted", status, func(entry *drand.DKGEntry) any {
			return formatAddresses(entry.Acceptors)
		})
		appendRow("Rejected", status, func(entry *drand.DKGEntry) any {
			return formatAddresses(entry.Rejectors)
		})
	}

	if dkg.Status(status.Current.State) >= dkg.Executing {
		appendRow("FinalGroup", status, func(entry *drand.DKGEntry) any {
			if entry.FinalGroup == nil {
				return ""
			}
			b := strings.Builder{}
			b.WriteString("[")
			for _, a := range entry.FinalGroup {
				b.WriteString(fmt.Sprintf("\n\t%s,", a))
			}
			b.WriteString("\n]")
			return b.String()
		})
	}

	fmt.Println(tw.Render())
}

func generateProposalCmd(c *cli.Context, l log.Logger) error {
	if !c.IsSet(joinerFlag.Name) && !c.IsSet(remainerFlag.Name) {
		return errors.New("you must add joiners and/or remainers to the proposal")
	}

	if !c.IsSet(proposalOutputFlag.Name) {
		return errors.New("you must pass an output filepath for the proposal")
	}

	var beaconID string
	if c.IsSet(beaconIDFlag.Name) {
		beaconID = c.String(beaconIDFlag.Name)
	} else {
		beaconID = common.DefaultBeaconID
	}

	p := ProposalFile{}

	fetchParticipantData := func(path string) (*drand.Participant, error) {
		parts := strings.Split(path, "https://")
		tls := len(parts) > 1
		var peer net.Peer
		if tls {
			peer = net.CreatePeer(parts[1], tls)
		} else {
			peer = net.CreatePeer(path, tls)
		}
		client := net.NewGrpcClient(l)
		identity, err := client.GetIdentity(context.Background(), peer, &drand.IdentityRequest{Metadata: &common2.Metadata{BeaconID: beaconID}})
		if err != nil {
			return nil, err
		}
		return &drand.Participant{
			Address:   identity.Address,
			Tls:       identity.Tls,
			PubKey:    identity.Key,
			Signature: identity.Signature,
		}, nil
	}

	for _, joiner := range c.StringSlice(joinerFlag.Name) {
		j, err := fetchParticipantData(joiner)
		if err != nil {
			return err
		}
		p.Joining = append(p.Joining, j)
	}

	for _, remainer := range c.StringSlice(remainerFlag.Name) {
		r, err := fetchParticipantData(remainer)
		if err != nil {
			return err
		}
		p.Remaining = append(p.Remaining, r)
	}

	for _, leaver := range c.StringSlice(leaverFlag.Name) {
		l, err := fetchParticipantData(leaver)
		if err != nil {
			return err
		}
		p.Leaving = append(p.Leaving, l)
	}

	filepath := c.String(proposalOutputFlag.Name)
	file, err := os.Create(filepath)
	if err != nil {
		return err
	}

	err = toml.NewEncoder(file).Encode(p.TOML())
	if err != nil {
		return err
	}

	fmt.Printf("Proposal created successfully at path %s", filepath)
	return nil
}
