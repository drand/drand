package drand

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/core"
	"github.com/drand/drand/v2/internal/dkg"
	"github.com/drand/drand/v2/internal/net"
	"github.com/drand/drand/v2/internal/util"
	drand "github.com/drand/drand/v2/protobuf/dkg"
	proto "github.com/drand/drand/v2/protobuf/drand"
)

var dkgGroupFlag = &cli.StringFlag{
	Name:    "group",
	Usage:   "The group file of the previous epoch",
	EnvVars: []string{"DRAND_DKG_GROUP"},
}

var dkgCommand = &cli.Command{
	Name:  "dkg",
	Usage: "Commands for interacting with the DKG",
	Subcommands: []*cli.Command{
		{
			Name: "init",
			Flags: toArray(
				beaconIDFlag,
				controlFlag,
				schemeFlag,
				periodFlag,
				thresholdFlag,
				catchupPeriodFlag,
				proposalFlag,
				dkgTimeoutFlag,
				genesisTimeFlag,
			),
			Action: func(c *cli.Context) error {
				l := log.New(nil, logLevel(c), logJSON(c)).
					Named("dkgInit")
				return dkgInit(c, l)
			},
		},
		{
			Name: "reshare",
			Flags: toArray(
				beaconIDFlag,
				controlFlag,
				thresholdFlag,
				catchupPeriodFlag,
				proposalFlag,
				dkgTimeoutFlag,
			),
			Action: func(c *cli.Context) error {
				l := log.New(nil, logLevel(c), logJSON(c)).
					Named("dkgReshare")
				return dkgReshare(c, l)
			},
		},
		{
			Name: "join",
			Flags: toArray(
				beaconIDFlag,
				controlFlag,
				dkgGroupFlag,
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
				controlFlag,
				leaverFlag,
			),
			Action: func(c *cli.Context) error {
				l := log.New(nil, logLevel(c), logJSON(c)).
					Named("dkgGenerateProposal")
				return generateProposalCmd(c, l)
			},
		},
		{
			Name: "nuke",
			Flags: toArray(
				beaconIDFlag,
				folderFlag,
			),
			Action: NukeDKGStateCmd,
		},
	},
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

var genesisTimeFlag = &cli.StringFlag{
	Name:  "genesis-delay",
	Usage: "The duration from now until the network should start creating randomness",
}

var dkgTimeoutFlag = &cli.StringFlag{
	Name:  "timeout",
	Usage: "The duration from now in which DKG participants should abort the DKG if it has not completed.",
	Value: "24h",
}

//nolint:dupl//not worth extracting a few lines
func dkgInit(c *cli.Context, l log.Logger) error {
	controlPort := withDefault(c.String(controlFlag.Name), core.DefaultControlPort)
	client, err := net.NewDKGControlClient(l, controlPort)
	if err != nil {
		return err
	}

	beaconID := withDefault(c.String(beaconIDFlag.Name), common.DefaultBeaconID)

	proposal, err := parseInitialProposal(c)
	if err != nil {
		return err
	}

	_, err = client.Command(c.Context, &drand.DKGCommand{
		Command: &drand.DKGCommand_Initial{Initial: proposal},
		Metadata: &drand.CommandMetadata{
			BeaconID: beaconID,
		},
	})
	if err != nil {
		return fmt.Errorf("DKG proposal was unsuccessful - you may need to issue an abort command. Error: %w", err)
	}

	fmt.Println("DKG initialized successfully!")

	return nil
}

//nolint:dupl//not worth extracting a few lines
func dkgReshare(c *cli.Context, l log.Logger) error {
	controlPort := withDefault(c.String(controlFlag.Name), core.DefaultControlPort)
	client, err := net.NewDKGControlClient(l, controlPort)
	if err != nil {
		return err
	}

	beaconID := withDefault(c.String(beaconIDFlag.Name), common.DefaultBeaconID)

	proposal, err := parseProposal(c)
	if err != nil {
		return err
	}

	_, err = client.Command(c.Context, &drand.DKGCommand{
		Command: &drand.DKGCommand_Resharing{Resharing: proposal},
		Metadata: &drand.CommandMetadata{
			BeaconID: beaconID,
		},
	})
	if err != nil {
		return fmt.Errorf("reshare proposal was unsuccessful - you may need to issue an abort command. Error: %w", err)
	}

	fmt.Println("Reshare proposed successfully!")
	return nil
}

func withDefault(first, second string) string {
	if first == "" {
		return second
	}

	return first
}

func parseInitialProposal(c *cli.Context) (*drand.FirstProposalOptions, error) {
	requiredFlags := []*cli.StringFlag{proposalFlag, periodFlag, schemeFlag, catchupPeriodFlag, genesisTimeFlag}

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

	genesisTime := time.Now().Add(c.Duration(genesisTimeFlag.Name))

	return &drand.FirstProposalOptions{
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

func parseProposal(c *cli.Context) (*drand.ProposalOptions, error) {
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

	return &drand.ProposalOptions{
		Timeout:              timestamppb.New(timeout),
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
	if c.IsSet(dkgGroupFlag.Name) {
		fileContents, err := os.ReadFile(c.String(dkgGroupFlag.Name))
		if err != nil {
			return err
		}
		groupFile = fileContents
	}

	client, err := net.NewDKGControlClient(l, controlPort)
	if err != nil {
		return err
	}
	_, err = client.Command(c.Context, &drand.DKGCommand{
		Command: &drand.DKGCommand_Join{Join: &drand.JoinOptions{
			GroupFile: groupFile,
		}},
		Metadata: &drand.CommandMetadata{
			BeaconID: beaconID,
		},
	})

	if err == nil {
		fmt.Println("Joined the DKG successfully!")
	}
	return err
}

func executeDKG(c *cli.Context) error {
	err := runSimpleAction(c, func(beaconID string, client drand.DKGControlClient) error {
		_, err := client.Command(c.Context, &drand.DKGCommand{
			Command: &drand.DKGCommand_Execute{Execute: &drand.ExecutionOptions{}},
			Metadata: &drand.CommandMetadata{
				BeaconID: beaconID,
			},
		})
		return err
	})

	if err == nil {
		fmt.Println("DKG execution started successfully!")
	}
	return err
}

func acceptDKG(c *cli.Context) error {
	err := runSimpleAction(c, func(beaconID string, client drand.DKGControlClient) error {
		_, err := client.Command(c.Context, &drand.DKGCommand{
			Command: &drand.DKGCommand_Accept{Accept: &drand.AcceptOptions{}},
			Metadata: &drand.CommandMetadata{
				BeaconID: beaconID,
			},
		})
		return err
	})

	if err == nil {
		fmt.Println("DKG accepted successfully!")
	}
	return err
}

func rejectDKG(c *cli.Context) error {
	err := runSimpleAction(c, func(beaconID string, client drand.DKGControlClient) error {
		_, err := client.Command(c.Context, &drand.DKGCommand{
			Command: &drand.DKGCommand_Reject{Reject: &drand.RejectOptions{}},
			Metadata: &drand.CommandMetadata{
				BeaconID: beaconID,
			},
		})
		return err
	})

	if err == nil {
		fmt.Println("DKG rejected successfully!")
	}

	return err
}

func abortDKG(c *cli.Context) error {
	err := runSimpleAction(c, func(beaconID string, client drand.DKGControlClient) error {
		_, err := client.Command(c.Context, &drand.DKGCommand{
			Command: &drand.DKGCommand_Abort{Abort: &drand.AbortOptions{}},
			Metadata: &drand.CommandMetadata{
				BeaconID: beaconID,
			},
		})
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
		"BeaconID:%s,State:%s,Epoch:%d,Threshold:%d,Timeout:%s,GenesisTime:%s,GenesisSeed:%s,Leader:%s",
		entry.BeaconID,
		dkg.Status(entry.State).String(),
		entry.Epoch,
		entry.Threshold,
		entry.Timeout.AsTime().String(),
		entry.GenesisTime.AsTime().String(),
		hex.EncodeToString(entry.GenesisSeed),
		entry.Leader.Address,
	)
	if err != nil {
		_, _ = fmt.Fprintln(out, "error")
	}
}

type printModel struct {
	Status      string
	BeaconID    string
	Epoch       string
	Threshold   string
	Timeout     string
	GenesisTime string
	GenesisSeed string
	Leader      string
	Joining     string
	Remaining   string
	Leaving     string
	Accepted    string
	Rejected    string
	FinalGroup  string
}

func formatAddresses(arr []*drand.Participant) string {
	if len(arr) == 0 {
		return "[]"
	}
	if len(arr) == 1 {
		return fmt.Sprintf("[\t%s\t]", arr[0].Address)
	}

	b := strings.Builder{}
	b.WriteString("[")

	for _, a := range arr {
		b.WriteString(fmt.Sprintf("\n\t%s,", a.Address))
	}
	b.WriteString("\n]")

	return b.String()
}

func addCheckmarks(arr []*drand.Participant, entry *drand.DKGEntry) []*drand.Participant {
	ret := make([]*drand.Participant, 0, len(arr))
	check := func(a *drand.Participant) bool {
		return util.Contains(entry.Remaining, a)
	}
	valid := func(a *drand.Participant) bool {
		return util.Contains(entry.Acceptors, a) || a.Address == entry.Leader.Address
	}
	invalid := func(a *drand.Participant) bool {
		return util.Contains(entry.Rejectors, a)
	}

	if len(entry.FinalGroup) > 0 {
		check = func(_ *drand.Participant) bool {
			return true
		}
		valid = func(a *drand.Participant) bool {
			return slices.Contains(entry.FinalGroup, a.Address)
		}
		invalid = func(a *drand.Participant) bool {
			return !slices.Contains(entry.FinalGroup, a.Address)
		}
	}
	for _, a := range arr {
		// we don't want any side effects, using a new list
		fresh := &drand.Participant{
			Address: a.Address,
		}
		if check(a) {
			checkmark := " ☐ "
			if valid(a) {
				checkmark = " ☑ "
			} else if invalid(a) {
				checkmark = " ☒ "
			}
			fresh.Address = checkmark + a.Address
		}
		ret = append(ret, fresh)
	}

	return ret
}

func formatFinalGroup(group []string) string {
	if group == nil {
		return ""
	}
	b := strings.Builder{}
	b.WriteString("[")
	for _, a := range group {
		b.WriteString(fmt.Sprintf("\n\t%s,", a))
	}
	b.WriteString("\n]")
	return b.String()
}

func convert(entry *drand.DKGEntry) printModel {
	if entry == nil {
		return printModel{}
	}

	return printModel{
		Status:      dkg.Status(entry.State).String(),
		BeaconID:    entry.BeaconID,
		Epoch:       strconv.Itoa(int(entry.Epoch)),
		Threshold:   strconv.Itoa(int(entry.Threshold)),
		Timeout:     entry.Timeout.AsTime().Format(time.RFC3339),
		GenesisTime: entry.GenesisTime.AsTime().Format(time.RFC3339),
		GenesisSeed: hex.EncodeToString(entry.GenesisSeed),
		Leader:      entry.Leader.Address,
		Joining:     formatAddresses(addCheckmarks(entry.Joining, entry)),
		Remaining:   formatAddresses(addCheckmarks(entry.Remaining, entry)),
		Leaving:     formatAddresses(entry.Leaving),
		Accepted:    formatAddresses(entry.Acceptors),
		Rejected:    formatAddresses(entry.Rejectors),
		FinalGroup:  formatFinalGroup(entry.FinalGroup),
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

	currentModel := convert(status.Current)
	finishedModel := convert(status.Complete)

	tw.AppendRow(table.Row{"Status", currentModel.Status, finishedModel.Status})
	tw.AppendRow(table.Row{"Epoch", currentModel.Epoch, finishedModel.Epoch})
	tw.AppendRow(table.Row{"BeaconID", currentModel.BeaconID, finishedModel.BeaconID})
	tw.AppendRow(table.Row{"Threshold", currentModel.Threshold, finishedModel.Threshold})
	tw.AppendRow(table.Row{"Timeout", currentModel.Timeout, finishedModel.Timeout})
	tw.AppendRow(table.Row{"GenesisTime", currentModel.GenesisTime, finishedModel.GenesisTime})
	tw.AppendRow(table.Row{"GenesisSeed", currentModel.GenesisSeed, finishedModel.GenesisSeed})
	tw.AppendRow(table.Row{"Leader", currentModel.Leader, finishedModel.Leader})
	tw.AppendRow(table.Row{"Joining", currentModel.Joining, finishedModel.Joining})
	tw.AppendRow(table.Row{"Remaining", currentModel.Remaining, finishedModel.Remaining})
	tw.AppendRow(table.Row{"Leaving", currentModel.Leaving, finishedModel.Leaving})
	tw.AppendRow(table.Row{"Accepted", currentModel.Accepted, finishedModel.Accepted})
	tw.AppendRow(table.Row{"Rejected", currentModel.Rejected, finishedModel.Rejected})
	tw.AppendRow(table.Row{"FinalGroup", currentModel.FinalGroup, finishedModel.FinalGroup})

	fmt.Println(tw.Render())
}

//nolint:funlen,gocyclo // this is a big function
func generateProposalCmd(c *cli.Context, l log.Logger) error {
	// first we validate the flags
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

	// then we fetch the current group file
	proposalFile := ProposalFile{}
	client, err := controlClient(c, l)
	if err != nil {
		return err
	}

	// if there is an existing group file, it's not a fresh start
	freshStart := false
	groupFile, err := client.GroupFile(beaconID)
	if err != nil {
		// if it's a fresh start, we'll take a different path
		if strings.Contains(err.Error(), core.ErrNoGroupSetup.Error()) {
			freshStart = true
		} else {
			return err
		}
	}

	joiners := c.StringSlice(joinerFlag.Name)
	remainers := c.StringSlice(remainerFlag.Name)
	leavers := c.StringSlice(leaverFlag.Name)

	if freshStart {
		if len(remainers) > 0 {
			return errors.New("the network isn't running yet - cannot have remainers")
		}
		if len(leavers) > 0 {
			return errors.New("the network isn't running yet - cannot have leavers")
		}
	}

	identityResp, err := client.PublicKey(beaconID)
	if err != nil {
		return err
	}
	sch, err := crypto.SchemeFromName(identityResp.GetSchemeName())
	if err != nil {
		return key.ErrInvalidKeyScheme
	}

	id, err := key.IdentityFromProto(&proto.Identity{
		Signature: identityResp.Signature,
		Address:   identityResp.Addr,
		Key:       identityResp.PubKey,
	}, sch)
	if err != nil {
		return key.ErrInvalidKeyScheme
	}

	if err = id.ValidSignature(); err != nil {
		return key.ErrInvalidKeyScheme
	}

	// we fetch the keys for all the new joiners by calling their node's API
	for _, joiner := range joiners {
		p, err := fetchPublicKey(beaconID, l, joiner, sch)
		if err != nil {
			return err
		}
		if p.Address != joiner {
			return fmt.Errorf("node %s returned a public key signed for %s", joiner, p.Address)
		}
		proposalFile.Joining = append(proposalFile.Joining, p)
	}

	// for migration from v1-v2, we need to actually get the updated public keys of the remainers and leavers
	// TODO: remove this post-migration
	for _, remainer := range remainers {
		p, err := fetchPublicKey(beaconID, l, remainer, sch)
		if err != nil {
			return err
		}
		if p.Address != remainer {
			return fmt.Errorf("node %s returned a public key signed for %s", remainer, p.Address)
		}
		proposalFile.Remaining = append(proposalFile.Remaining, p)
	}

	// a best-effort shot at getting leavers' keys, just to simplify some of the verification of participants
	// later in the protocol
	for _, leaver := range leavers {
		// first we try and contact the leaver directly
		p, err := fetchPublicKey(beaconID, l, leaver, sch)
		if err != nil {
			// if there is an error contacting them, we pull their public key from the old group file
			l.Warnw("could not fetch public key for leaver", "leaver", leaver)
			p, err = keyFromGroupFile(leaver, groupFile)
			if err != nil {
				return err
			}
		}
		proposalFile.Leaving = append(proposalFile.Leaving, p)
	}

	// finally we write the proposal toml file to the output location
	filepath := c.String(proposalOutputFlag.Name)
	file, err := os.Create(filepath)
	if err != nil {
		return err
	}

	err = toml.NewEncoder(file).Encode(proposalFile.TOML())
	if err != nil {
		return err
	}

	fmt.Printf("Proposal created successfully at path %s\n", filepath)
	return nil
}

func keyFromGroupFile(address string, groupFile *proto.GroupPacket) (*drand.Participant, error) {
	for _, node := range groupFile.Nodes {
		if node.Public.Address != address {
			continue
		}
		return util.ToParticipant(node), nil
	}

	return nil, fmt.Errorf("address %s was not found in the group file", address)
}

func fetchPublicKey(beaconID string, l log.Logger, address string, targetSch *crypto.Scheme) (*drand.Participant, error) {
	peer := net.CreatePeer(address)
	client := net.NewGrpcClient(l)
	identity, err := client.GetIdentity(context.Background(), peer, &proto.IdentityRequest{Metadata: &proto.Metadata{BeaconID: beaconID}})
	if err != nil {
		return nil, fmt.Errorf("could not fetch public key for %s: %w", address, err)
	}

	if identity.SchemeName != targetSch.Name {
		return nil, key.ErrInvalidKeyScheme
	}

	part := &drand.Participant{
		Address:   identity.Address,
		Key:       identity.Key,
		Signature: identity.Signature,
	}

	id, err := key.IdentityFromProto(part, targetSch)
	if err != nil {
		return nil, err
	}
	l.Debugw("Validating signature for", id.Addr, "scheme", targetSch.Name)
	if err := id.ValidSignature(); err != nil {
		return nil, key.ErrInvalidKeyScheme
	}

	return part, nil
}
