// Package drand is a distributed randomness beacon. It provides periodically an
// unpredictable, bias-resistant, and verifiable random value.
package drand

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	gonet "net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/urfave/cli/v2"

	common3 "github.com/drand/drand/common"
	"github.com/drand/drand/common/key"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/internal/chain"
	"github.com/drand/drand/internal/chain/boltdb"
	"github.com/drand/drand/internal/core"
	"github.com/drand/drand/internal/core/migration"
	"github.com/drand/drand/internal/fs"
	"github.com/drand/drand/internal/net"
	common2 "github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.buildDate=$(date -u +%d/%m/%Y@%H:%M:%S) -X main.gitCommit=$(git rev-parse HEAD)"
var (
	gitCommit = "none"
	buildDate = "unknown"
)

var SetVersionPrinter sync.Once

const defaultPort = "8080"

func banner(w io.Writer) {
	version := common3.GetAppVersion()
	_, _ = fmt.Fprintf(w, "drand %s (date %v, commit %v)\n", version.String(), buildDate, gitCommit)
}

var folderFlag = &cli.StringFlag{
	Name:    "folder",
	Value:   core.DefaultConfigFolder(),
	Usage:   "Folder to keep all drand cryptographic information, with absolute path.",
	EnvVars: []string{"DRAND_FOLDER"},
}

var verboseFlag = &cli.BoolFlag{
	Name:    "verbose",
	Usage:   "If set, verbosity is at the debug level",
	EnvVars: []string{"DRAND_VERBOSE"},
}

var tlsCertFlag = &cli.StringFlag{
	Name: "tls-cert",
	Usage: "Set the TLS certificate chain (in PEM format) for this drand node. " +
		"The certificates have to be specified as a list of whitespace-separated file paths. " +
		"This parameter is required by default and can only be omitted if the --tls-disable flag is used.",
	EnvVars: []string{"DRAND_TLS_CERT"},
}

var tlsKeyFlag = &cli.StringFlag{
	Name: "tls-key",
	Usage: "Set the TLS private key (in PEM format) for this drand node. " +
		"The key has to be specified as a file path. " +
		"This parameter is required by default and can only be omitted if the --tls-disable flag is used.",
	EnvVars: []string{"DRAND_TLS_KEY"},
}

var insecureFlag = &cli.BoolFlag{
	Name:    "tls-disable",
	Aliases: []string{"insecure"},
	Usage:   "Disable TLS for all communications (not recommended).",
	EnvVars: []string{"DRAND_TLS_DISABLE", "DRAND_INSECURE"},
}

var controlFlag = &cli.StringFlag{
	Name:    "control",
	Usage:   "Set the port you want to listen to for control port commands. If not specified, we will use the default value.",
	Value:   "8888",
	EnvVars: []string{"DRAND_CONTROL"},
}

var metricsFlag = &cli.StringFlag{
	Name:    "metrics",
	Usage:   "Launch a metrics server at the specified (host:)port.",
	EnvVars: []string{"DRAND_METRICS"},
}

var tracesFlag = &cli.StringFlag{
	Name:    "traces",
	Usage:   "Publish metrics to the specific OpenTelemetry compatible host:port server. E.g. 127.0.0.1:4317",
	EnvVars: []string{"DRAND_TRACES"},
}

var tracesProbabilityFlag = &cli.Float64Flag{
	Name: "traces-probability",
	Usage: "The probability for a certain trace to end up being collected." +
		"Between 0.0 and 1.0 values, that corresponds to 0% and 100%." +
		"Be careful as a high probability ratio can produce a lot of data.",
	EnvVars: []string{"DRAND_TRACES_PROBABILITY"},
	Value:   0.05,
}

var privListenFlag = &cli.StringFlag{
	Name:    "private-listen",
	Usage:   "Set the listening (binding) address of the private API. Useful if you have some kind of proxy.",
	EnvVars: []string{"DRAND_PRIVATE_LISTEN"},
}

var pubListenFlag = &cli.StringFlag{
	Name:    "public-listen",
	Usage:   "Set the listening (binding) address of the public API. Useful if you have some kind of proxy.",
	EnvVars: []string{"DRAND_PUBLIC_LISTEN"},
}

var nodeFlag = &cli.StringFlag{
	Name:    "nodes",
	Usage:   "Contact the nodes at the given list of whitespace-separated addresses which have to be present in group.toml.",
	EnvVars: []string{"DRAND_NODES"},
}

var roundFlag = &cli.IntFlag{
	Name: "round",
	Usage: "Request the public randomness generated at round num. If the drand beacon does not have the requested value," +
		" it returns an error. If not specified, the current randomness is returned.",
	EnvVars: []string{"DRAND_ROUND"},
}

var certsDirFlag = &cli.StringFlag{
	Name:    "certs-dir",
	Usage:   "directory containing trusted certificates (PEM format). Useful for testing and self signed certificates",
	EnvVars: []string{"DRAND_CERTS_DIR"},
}

var outFlag = &cli.StringFlag{
	Name:    "out",
	Usage:   "save the group file into a separate file instead of stdout",
	EnvVars: []string{"DRAND_OUT"},
}

var periodFlag = &cli.StringFlag{
	Name:    "period",
	Usage:   "period to set when doing a setup",
	EnvVars: []string{"DRAND_PERIOD"},
}

var catchupPeriodFlag = &cli.StringFlag{
	Name:    "catchup-period",
	Usage:   "Minimum period while in catchup. Set only by the leader of share / reshares",
	Value:   "0s",
	EnvVars: []string{"DRAND_CATCHUP_PERIOD"},
}

var thresholdFlag = &cli.IntFlag{
	Name:    "threshold",
	Usage:   "threshold to use for the DKG",
	EnvVars: []string{"DRAND_THRESHOLD"},
}

// TODO (dlsniper): This flag is a duplicate name of the nodeFlag. Should change the name.
var shareNodeFlag = &cli.IntFlag{
	Name:    "nodes",
	Usage:   "number of nodes expected",
	EnvVars: []string{"DRAND_NODES"},
}

var transitionFlag = &cli.BoolFlag{
	Name:    "reshare",
	Aliases: []string{"transition"},
	Usage: "When set, this flag indicates the share operation is a resharing. " +
		"The node will use the currently stored group as the basis for the resharing",
	EnvVars: []string{"DRAND_TRANSITION_FLAG"},
}

var forceFlag = &cli.BoolFlag{
	Name:    "force",
	Aliases: []string{"f"},
	Usage: "When set, this flag forces the daemon to start a new reshare operation. " +
		"By default, it does not allow to restart one",
	EnvVars: []string{"DRAND_FORCE"},
}

// secretFlag is the "manual" security when the "leader"/coordinator creates the
// group: every participant must know this secret. It is not a consensus, not
// perfect, but since all members are known after the protocol, and members can
// decide to redo the setup, it works in practice well enough.
// TODO Add a manual check when the group is created so the user manually ACK.
var secretFlag = &cli.StringFlag{
	Name: "secret-file",
	Usage: "Specify the secret to use when doing the share so the leader knows you are an eligible potential participant." +
		" must be at least 32 characters.",
	EnvVars: []string{"DRAND_SECRET_FILE"},
}

var connectFlag = &cli.StringFlag{
	Name:    "connect",
	Usage:   "Address of the coordinator that will assemble the public keys and start the DKG",
	EnvVars: []string{"DRAND_CONNECT"},
}

var leaderFlag = &cli.BoolFlag{
	Name:    "leader",
	Usage:   "Specify if this node should act as the leader for setting up the group",
	EnvVars: []string{"DRAND_LEADER"},
}

var beaconOffset = &cli.IntFlag{
	Name: "beacon-delay",
	Usage: "Leader uses this flag to specify the genesis time or transition time as a delay from when " +
		" group is ready to run the share protocol",
	EnvVars: []string{"DRAND_BEACON_DELAY"},
}

var oldGroupFlag = &cli.StringFlag{
	Name: "from",
	Usage: "Old group.toml path to specify when a new node wishes to participate " +
		"in a resharing protocol. This flag is optional in case a node is already" +
		"included in the current DKG.",
	EnvVars: []string{"DRAND_FROM"},
}

var proposalFlag = &cli.StringFlag{
	Name:    "proposal",
	Usage:   "Path to a toml file specifying the leavers, joiners and remainers for a network proposal",
	EnvVars: []string{"DRAND_PROPOSAL_PATH"},
}

var skipValidationFlag = &cli.BoolFlag{
	Name:    "skipValidation",
	Usage:   "skips bls verification of beacon rounds for faster catchup.",
	EnvVars: []string{"DRAND_SKIP_VALIDATION"},
}

var timeoutFlag = &cli.StringFlag{
	Name:    "timeout",
	Usage:   fmt.Sprintf("Timeout to use during the DKG, in string format. Default is %s", core.DefaultDKGPhaseTimeout),
	EnvVars: []string{"DRAND_TIMEOUT"},
}

var pushFlag = &cli.BoolFlag{
	Name: "push",
	Usage: "Push mode forces the daemon to start making beacon requests to the other node, " +
		"instead of waiting the other nodes contact it to catch-up on the round",
	EnvVars: []string{"DRAND_PUSH"},
}

var sourceFlag = &cli.StringFlag{
	Name:    "source",
	Usage:   "Source flag allows to provide an executable which output will be used as additional entropy during resharing step.",
	EnvVars: []string{"DRAND_SOURCE"},
}

var userEntropyOnlyFlag = &cli.BoolFlag{
	Name: "user-source-only",
	Usage: "user-source-only flag used with the source flag allows to only use the user's entropy to pick the dkg secret " +
		"(won't be mixed with crypto/rand). Should be used for reproducibility and debbuging purposes.",
	EnvVars: []string{"DRAND_USER_SOURCE_ONLY"},
}

var groupFlag = &cli.StringFlag{
	Name:    "group",
	Usage:   "Test connections to nodes listed in the group",
	EnvVars: []string{"DRAND_GROUP"},
}

var hashOnly = &cli.BoolFlag{
	Name:    "hash",
	Usage:   "Only print the hash of the group file",
	EnvVars: []string{"DRAND_HASH"},
}

var hashInfoReq = &cli.StringFlag{
	Name:     "chain-hash",
	Usage:    "The hash of the chain info, used to validate integrity of the received group info",
	Required: true,
	EnvVars:  []string{"DRAND_CHAIN_HASH"},
}

// TODO (DLSNIPER): This is a duplicate of the hashInfoReq. Should these be merged into a single flag?
var hashInfoNoReq = &cli.StringFlag{
	Name:    "chain-hash",
	Usage:   "The hash of the chain info",
	EnvVars: []string{"DRAND_CHAIN_HASH"},
}

// using a simple string flag because the StringSliceFlag is not intuitive
// see https://github.com/urfave/cli/issues/62
var syncNodeFlag = &cli.StringFlag{
	Name: "sync-nodes",
	Usage: "<ADDRESS:PORT>,<...> of (multiple) reachable drand daemon(s). " +
		"When checking our local database, using our local daemon address will result in a dry run.",
	Required: true,
	EnvVars:  []string{"DRAND_SYNC_NODES"},
}

var followFlag = &cli.BoolFlag{
	Name: "follow",
	Usage: "Indicates whether we want to follow another daemon, if not we perform a check of our local DB. " +
		"Requires to specify the chain-hash using the '" + hashInfoNoReq.Name + "' flag.",
	EnvVars: []string{"DRAND_FOLLOW"},
}

var upToFlag = &cli.IntFlag{
	Name: "up-to",
	Usage: "Specify a round at which the drand daemon will stop syncing the chain, " +
		"typically used to bootstrap a new node in chained mode",
	Value:   0,
	EnvVars: []string{"DRAND_UP_TO"},
}

var schemeFlag = &cli.StringFlag{
	Name:    "scheme",
	Usage:   "Indicates a set of values drand will use to configure the randomness generation process",
	Value:   crypto.DefaultSchemeID,
	EnvVars: []string{"DRAND_SCHEME"},
}

var jsonFlag = &cli.BoolFlag{
	Name:    "json",
	Usage:   "Set the output as json format",
	EnvVars: []string{"DRAND_JSON"},
}

var beaconIDFlag = &cli.StringFlag{
	Name:    "id",
	Usage:   "Indicates the id for the randomness generation process which will be started",
	Value:   "",
	EnvVars: []string{"DRAND_ID"},
}
var listIdsFlag = &cli.BoolFlag{
	Name:    "list-ids",
	Usage:   "Indicates if it only have to list the running beacon ids instead of the statuses.",
	Value:   false,
	EnvVars: []string{"DRAND_LIST_IDS"},
}

var allBeaconsFlag = &cli.BoolFlag{
	Name:    "all",
	Usage:   "Indicates if we have to interact with all beacons chains",
	Value:   false,
	EnvVars: []string{"DRAND_ALL"},
}

var storageTypeFlag = &cli.StringFlag{
	Name:    "db",
	Usage:   "Which database engine to use. Supported values: bolt, postgres, or memdb.",
	Value:   "bolt",
	EnvVars: []string{"DRAND_DB"},
}

var pgDSNFlag = &cli.StringFlag{
	Name: "pg-dsn",
	Usage: "PostgreSQL DSN configuration.\n" +
		"Supported options are:\n" +
		//nolint:lll
		"- sslmode: if the SSL connection is disabled or required. Default disabled. See: https://www.postgresql.org/docs/15/libpq-ssl.html#LIBPQ-SSL-PROTECTION\n" +
		//nolint:lll
		"- connect_timeout: how many seconds before the connection attempt times out. Default 5 (seconds). See: https://www.postgresql.org/docs/15/libpq-connect.html#LIBPQ-CONNECT-CONNECT-TIMEOUT\n" +
		"- max-idle: number of maximum idle connections. Default: 2\n" +
		"- max-open: number of maximum open connections. Default: 0 - unlimited.\n",

	Value:   "postgres://drand:drand@127.0.0.1:5432/drand?sslmode=disable&connect_timeout=5",
	EnvVars: []string{"DRAND_PG_DSN"},
}

var memDBSizeFlag = &cli.IntFlag{
	Name:    "memdb-size",
	Usage:   "The buffer size for in-memory storage. Must be at least 10. Recommended, 2000 or more",
	Value:   2000,
	EnvVars: []string{"DRAND_MEMDB_SIZE"},
}

var appCommands = []*cli.Command{
	dkgCommand,
	{
		Name:  "start",
		Usage: "Start the drand daemon.",
		Flags: toArray(folderFlag, tlsCertFlag, tlsKeyFlag,
			insecureFlag, controlFlag, privListenFlag, pubListenFlag,
			metricsFlag, tracesFlag, tracesProbabilityFlag,
			certsDirFlag, pushFlag, verboseFlag, oldGroupFlag,
			skipValidationFlag, jsonFlag, beaconIDFlag,
			storageTypeFlag, pgDSNFlag, memDBSizeFlag),
		Action: func(c *cli.Context) error {
			banner(c.App.Writer)
			l := log.New(nil, logLevel(c), logJSON(c)).
				Named("startCmd")
			return startCmd(c, l)
		},
		Before: func(c *cli.Context) error {
			l := log.New(nil, logLevel(c), logJSON(c)).
				Named("runMigrationCmd")
			return runMigration(c, l)
		},
	},
	{
		Name:  "stop",
		Usage: "Stop the drand daemon.\n",
		Flags: toArray(controlFlag, beaconIDFlag),
		Action: func(c *cli.Context) error {
			banner(c.App.Writer)
			l := log.New(nil, logLevel(c), logJSON(c)).
				Named("stopDaemon")
			return stopDaemon(c, l)
		},
	},
	{
		Name:  "share",
		Usage: "Launch a sharing protocol.",
		Flags: toArray(insecureFlag, controlFlag, oldGroupFlag,
			timeoutFlag, sourceFlag, userEntropyOnlyFlag, secretFlag,
			periodFlag, shareNodeFlag, thresholdFlag, connectFlag, outFlag,
			leaderFlag, beaconOffset, transitionFlag, forceFlag, catchupPeriodFlag,
			schemeFlag, beaconIDFlag),
		Action: func(c *cli.Context) error {
			banner(c.App.Writer)
			return deprecatedShareCommand(c)
		},
	},
	{
		Name:  "load",
		Usage: "Launch a sharing protocol from filesystem",
		Flags: toArray(controlFlag, beaconIDFlag, insecureFlag),
		Action: func(c *cli.Context) error {
			l := log.New(nil, logLevel(c), logJSON(c)).
				Named("loadCmd")
			return loadCmd(c, l)
		},
	},
	{
		Name: "sync",
		Usage: "sync your local randomness chain with other nodes and validate your local beacon chain. To follow a " +
			"remote node, it requires the use of the '" + followFlag.Name + "' flag.",
		Flags: toArray(folderFlag, controlFlag, hashInfoNoReq, syncNodeFlag,
			tlsCertFlag, insecureFlag, upToFlag, beaconIDFlag, followFlag),
		Action: func(c *cli.Context) error {
			l := log.New(nil, logLevel(c), logJSON(c)).
				Named("syncCmd")
			return syncCmd(c, l)
		},
	},
	{
		Name: "generate-keypair",
		Usage: "Generate the longterm keypair (drand.private, drand.public) " +
			"for this node, and load it on the drand daemon if it is up and running.\n",
		ArgsUsage: "<address> is the address other nodes will be able to contact this node on (specified as 'private-listen' to the daemon)",
		Flags:     toArray(controlFlag, folderFlag, insecureFlag, beaconIDFlag, schemeFlag),
		Action: func(c *cli.Context) error {
			banner(c.App.Writer)
			l := log.New(nil, logLevel(c), logJSON(c)).
				Named("generateKeyPairCmd")

			err := keygenCmd(c, l)

			// If keys were generated successfully, daemon needs to load them
			// In other to load them, we run LoadBeacon cmd.
			//
			// TIP: If an error is found, it may indicate daemon is not running. If that is the case, keys will be loaded
			// on drand startup.
			if err == nil {
				err2 := loadCmd(c, l)
				if err2 != nil {
					fmt.Fprintf(os.Stdout, "Keys couldn't be loaded on drand daemon. If it is not running, "+
						"these new keys will be loaded on startup. Err: %s\n", err2)
				}
			}
			return err
		},
		Before: func(c *cli.Context) error {
			l := log.New(nil, logLevel(c), logJSON(c)).
				Named("generateKeyPairCmd")
			return checkMigration(c, l)
		},
	},

	{
		Name: "get",
		Usage: "get allows for public information retrieval from a remote " +
			"drand node.\n",
		Subcommands: []*cli.Command{
			{
				Name: "public",
				Usage: "Get the latest public randomness from the drand " +
					"beacon and verify it against the collective public key " +
					"as specified in group.toml. Only one node is contacted by " +
					"default. This command attempts to connect to the drand " +
					"beacon via TLS and falls back to plaintext communication " +
					"if the contacted node has not activated TLS in which case " +
					"it prints a warning.\n",
				Flags: toArray(tlsCertFlag, insecureFlag, roundFlag, nodeFlag),
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("getPublicRandomness")
					return getPublicRandomness(c, l)
				},
			},
			{
				Name:      "chain-info",
				Usage:     "Get the binding chain information that this node participates to",
				ArgsUsage: "`ADDRESS1` `ADDRESS2` ... provides the addresses of the node to try to contact to.",
				Flags:     toArray(tlsCertFlag, insecureFlag, hashOnly, hashInfoNoReq),
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("getChainInfo")
					return getChainInfo(c, l)
				},
			},
		},
	},
	{
		Name:  "util",
		Usage: "Multiple commands of utility functions, such as reseting a state, checking the connection of a peer...",
		Subcommands: []*cli.Command{
			{
				Name: "check",
				Usage: "Check node at the given `ADDRESS` (you can put multiple ones)" +
					" in the group for accessibility over the gRPC communication. If the node " +
					" is not running behind TLS, you need to pass the tls-disable flag. You can " +
					"also check a whole group's connectivity with the group flag.",
				Flags: toArray(groupFlag, certsDirFlag, insecureFlag, verboseFlag, beaconIDFlag),
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("checkConnection")
					return checkConnection(c, l)
				},
				Before: checkArgs,
			},
			{
				Name: "remote-status",
				Usage: "Ask for the statuses of remote nodes indicated by " +
					"`ADDRESS1 ADDRESS2 ADDRESS3...`, including the network " +
					"visibility over the rest of the addresses given.",
				Flags: toArray(controlFlag, jsonFlag, beaconIDFlag),
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("remoteStatusCmd")
					return remoteStatusCmd(c, l)
				},
			},
			{
				Name:  "ping",
				Usage: "Pings the daemon checking its state\n",
				Flags: toArray(controlFlag),
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("pingpongCmd")
					return pingpongCmd(c, l)
				},
			},
			{
				Name:  "list-schemes",
				Usage: "List all scheme ids available to use\n",
				Flags: toArray(controlFlag),
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("schemesCmd")
					return schemesCmd(c, l)
				},
			},
			{
				Name:  "status",
				Usage: "Get the status of many modules of running the daemon\n",
				Flags: toArray(controlFlag, jsonFlag, beaconIDFlag, allBeaconsFlag, listIdsFlag),
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("statusCmd")
					return statusCmd(c, l)
				},
			},
			{
				Name:  "migrate",
				Usage: "Migrate folder structure to support multi-beacon drand. You DO NOT have to run it while drand is running.\n",
				Flags: toArray(folderFlag),
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("migrateCmd")
					return migrateCmd(c, l)
				},
				Before: checkArgs,
			},
			{
				Name:  "reset",
				Usage: "Resets the local distributed information (share, group file and random beacons). It KEEPS the private/public key pair.",
				Flags: toArray(folderFlag, controlFlag, beaconIDFlag, allBeaconsFlag),
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("resetCmd")
					return resetCmd(c, l)
				},
				Before: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("resetCmd")
					return checkMigration(c, l)
				},
			},
			{
				Name: "del-beacon",
				Usage: "Delete all beacons from the given `ROUND` number until the head of the chain. " +
					" You MUST restart the daemon after that command.",
				Flags: toArray(folderFlag, beaconIDFlag, allBeaconsFlag),
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("deleteBeaconCmd")
					return deleteBeaconCmd(c, l)
				},
				Before: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("deleteBeaconCmd")
					return checkMigration(c, l)
				},
			},
			{
				Name:  "self-sign",
				Usage: "Signs the public identity of this node. Needed for backward compatibility with previous versions.",
				Flags: toArray(folderFlag, beaconIDFlag),
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("selfSignCmd")
					return selfSign(c, l)
				},
				Before: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("selfSignCmd")
					return checkMigration(c, l)
				},
			},
			{
				Name:  "backup",
				Usage: "backs up the primary drand database to a secondary location.",
				Flags: toArray(outFlag, controlFlag, beaconIDFlag),
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("backupDBCmd")
					return backupDBCmd(c, l)
				},
			},
		},
	},
	{
		Name: "show",
		Usage: "local information retrieval about the node's cryptographic " +
			"material. Show prints the information about the collective " +
			"public key (drand.cokey), the group details (group.toml)," +
			"the long-term public key " +
			"(drand.public), or the private key share (drand.share), " +
			"respectively.\n",
		Flags: toArray(folderFlag, controlFlag),
		Subcommands: []*cli.Command{
			{
				Name: "group",
				Usage: "shows the current group.toml used. The group.toml " +
					"may contain the distributed public key if the DKG has been " +
					"ran already.\n",
				Flags: toArray(outFlag, controlFlag, hashOnly, beaconIDFlag),
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("showGroupCmd")
					return showGroupCmd(c, l)
				},
			},
			{
				Name:  "chain-info",
				Usage: "shows the chain information this node is participating to",
				Flags: toArray(controlFlag, hashOnly, beaconIDFlag),
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("showChainInfoCmd")
					return showChainInfo(c, l)
				},
			},
			{
				Name:  "public",
				Usage: "shows the long-term public key of a node.\n",
				Flags: toArray(controlFlag, beaconIDFlag),
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("showPublicCmd")
					return showPublicCmd(c, l)
				},
			},
		},
	},
}

// CLI runs the drand app
func CLI() *cli.App {
	version := common3.GetAppVersion()

	app := cli.NewApp()
	app.Name = "drand"

	// See https://cli.urfave.org/v2/examples/bash-completions/#enabling for how to turn on.
	app.EnableBashCompletion = true

	SetVersionPrinter.Do(func() {
		cli.VersionPrinter = func(c *cli.Context) {
			fmt.Fprintf(c.App.Writer, "drand %s (date %v, commit %v)\n", version, buildDate, gitCommit)
		}
	})

	app.ExitErrHandler = func(context *cli.Context, err error) {
		// override to prevent default behavior of calling OS.exit(1),
		// when tests expect to be able to run multiple commands.
	}
	app.Version = version.String()
	app.Usage = "distributed randomness service"
	// =====Commands=====
	// we need to copy the underlying commands to avoid races, cli sadly doesn't support concurrent executions well
	appComm := make([]*cli.Command, len(appCommands))
	for i, p := range appCommands {
		if p == nil {
			continue
		}
		v := *p
		appComm[i] = &v
	}
	app.Commands = appComm
	// we need to copy the underlying flags to avoid races
	verbFlag := *verboseFlag
	foldFlag := *folderFlag
	app.Flags = toArray(&verbFlag, &foldFlag)
	app.Before = testWindows
	return app
}

func resetCmd(c *cli.Context, l log.Logger) error {
	conf := contextToConfig(c, l)

	fmt.Fprintf(c.App.Writer, "You are about to delete your local share, group file and generated random beacons. "+
		"Are you sure you wish to perform this operation? [y/N]")
	reader := bufio.NewReader(c.App.Reader)

	answer, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading: %w", err)
	}

	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" {
		fmt.Fprintf(c.App.Writer, "drand: not reseting the state.")
		return nil
	}

	stores, err := getKeyStores(c, l)
	if err != nil {
		fmt.Fprintf(c.App.Writer, "drand: err reading beacons database: %v\n", err)
		os.Exit(1)
	}

	for beaconID, store := range stores {
		if err := store.Reset(); err != nil {
			fmt.Fprintf(c.App.Writer, "drand: beacon id [%s] - err reseting key store: %v\n", beaconID, err)
			os.Exit(1)
		}

		if err := os.RemoveAll(path.Join(conf.ConfigFolderMB(), beaconID)); err != nil {
			fmt.Fprintf(c.App.Writer, "drand: beacon id [%s] - err reseting beacons database: %v\n", beaconID, err)
			os.Exit(1)
		}

		fmt.Printf("drand: beacon id [%s] - database reset\n", beaconID)
	}

	return nil
}

func askPort(c *cli.Context) string {
	for {
		fmt.Fprintf(c.App.Writer, "No valid port given. Please, choose a port number (or ENTER for default port 8080): ")

		reader := bufio.NewReader(c.App.Reader)
		input, err := reader.ReadString('\n')
		if err != nil {
			continue
		}

		portStr := strings.TrimSpace(input)
		if portStr == "" {
			fmt.Fprintln(c.App.Writer, "Default port selected")
			return defaultPort
		}

		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1000 || port > 65536 {
			continue
		}

		return portStr
	}
}

func runMigration(c *cli.Context, l log.Logger) error {
	if err := checkArgs(c); err != nil {
		return err
	}

	config := contextToConfig(c, l)

	if err := migration.MigrateSBFolderStructure(config.ConfigFolder()); err != nil {
		return err
	}

	return nil
}

func checkMigration(c *cli.Context, l log.Logger) error {
	if err := checkArgs(c); err != nil {
		return err
	}

	config := contextToConfig(c, l)

	if isPresent := migration.CheckSBFolderStructure(config.ConfigFolder()); isPresent {
		return fmt.Errorf("single-beacon drand folder structure was not migrated, " +
			"please first do it with 'drand util migrate' command")
	}

	if fs.CreateSecureFolder(config.ConfigFolderMB()) == "" {
		return fmt.Errorf("something went wrong with the multi beacon folder. " +
			"Make sure that you have the appropriate rights")
	}

	return nil
}

func testWindows(c *cli.Context) error {
	// x509 not available on windows: must run without TLS
	if runtime.GOOS == "windows" && !c.Bool(insecureFlag.Name) {
		return errors.New("TLS is not available on Windows, please disable TLS")
	}
	return nil
}

func keygenCmd(c *cli.Context, l log.Logger) error {
	args := c.Args()
	if !args.Present() {
		return errors.New("missing drand address in argument. Abort")
	}

	if args.Len() > 1 {
		return fmt.Errorf("expecting only one argument, the address, but got:"+
			"\n\t%v\nAborting. Note that the flags need to go before the argument", args.Slice())
	}

	addr := args.First()
	var validID = regexp.MustCompile(`:\d+$`)
	if !validID.MatchString(addr) {
		fmt.Println("Invalid port:", addr)
		addr = addr + ":" + askPort(c)
	}

	sch, err := crypto.SchemeFromName(c.String(schemeFlag.Name))
	if err != nil {
		return err
	}

	var priv *key.Pair
	if c.Bool(insecureFlag.Name) {
		fmt.Println("Generating private / public key pair without TLS.")
		priv, err = key.NewKeyPair(addr, sch)
	} else {
		fmt.Println("Generating private / public key pair with TLS indication")
		priv, err = key.NewTLSKeyPair(addr, sch)
	}
	if err != nil {
		return err
	}

	config := contextToConfig(c, l)
	beaconID := getBeaconID(c)
	fileStore := key.NewFileStore(config.ConfigFolderMB(), beaconID)

	if _, err := fileStore.LoadKeyPair(sch); err == nil {
		keyDirectory := path.Join(config.ConfigFolderMB(), beaconID)
		fmt.Fprintf(c.App.Writer, "Keypair already present in `%s`.\nRemove them before generating new one\n", keyDirectory)
		return nil
	}
	if err := fileStore.SaveKeyPair(priv); err != nil {
		return fmt.Errorf("could not save key: %w", err)
	}

	fullpath := path.Join(config.ConfigFolderMB(), beaconID, key.FolderName)
	absPath, err := filepath.Abs(fullpath)

	if err != nil {
		return fmt.Errorf("err getting full path: %w", err)
	}
	fmt.Println("Generated keys at ", absPath)

	var buff bytes.Buffer
	if err := toml.NewEncoder(&buff).Encode(priv.Public.TOML()); err != nil {
		return err
	}

	buff.WriteString("\n")
	fmt.Println(buff.String())
	return nil
}

func groupOut(c *cli.Context, group *key.Group) error {
	if c.IsSet("out") {
		groupPath := c.String("out")
		if err := key.Save(groupPath, group, false); err != nil {
			return fmt.Errorf("drand: can't save group to specified file name: %w", err)
		}
	} else if c.Bool(hashOnly.Name) {
		fmt.Fprintf(c.App.Writer, "%x\n", group.Hash())
	} else {
		var buff bytes.Buffer
		if err := toml.NewEncoder(&buff).Encode(group.TOML()); err != nil {
			return fmt.Errorf("drand: can't encode group to TOML: %w", err)
		}
		buff.WriteString("\n")
		fmt.Fprintf(c.App.Writer, "The following group.toml file has been created\n")
		fmt.Fprint(c.App.Writer, buff.String())
		fmt.Fprintf(c.App.Writer, "\nHash of the group configuration: %x\n", group.Hash())
	}
	return nil
}

func checkConnection(c *cli.Context, lg log.Logger) error {
	var names []string
	var beaconID string

	if c.IsSet(groupFlag.Name) {
		if c.IsSet(beaconIDFlag.Name) {
			return fmt.Errorf("id flag is not reqired when using group flag")
		}
		if err := testEmptyGroup(c.String(groupFlag.Name)); err != nil {
			return err
		}
		group := new(key.Group)
		if err := key.Load(c.String(groupFlag.Name), group); err != nil {
			return fmt.Errorf("loading group failed: %w", err)
		}

		for _, id := range group.Nodes {
			names = append(names, id.Address())
		}
		beaconID = common3.GetCanonicalBeaconID(group.ID)
	} else if c.Args().Present() {
		for _, serverAddr := range c.Args().Slice() {
			_, _, err := gonet.SplitHostPort(serverAddr)
			if err != nil {
				return fmt.Errorf("error for address %s: %w", serverAddr, err)
			}
			names = append(names, serverAddr)
		}
		beaconID = common3.GetCanonicalBeaconID(c.String(beaconIDFlag.Name))
	} else {
		return fmt.Errorf("drand: check-group expects a list of identities or %s flag", groupFlag.Name)
	}

	conf := contextToConfig(c, lg)
	isVerbose := c.IsSet(verboseFlag.Name)
	allGood := true
	isIdentityCheck := c.IsSet(groupFlag.Name) || c.IsSet(beaconIDFlag.Name)
	invalidIds := make([]string, 0)

	for _, address := range names {
		var err error
		if isIdentityCheck {
			err = checkIdentityAddress(lg, conf, address, !c.Bool(insecureFlag.Name), beaconID)
		} else {
			err = remotePingToNode(lg, address, !c.Bool(insecureFlag.Name))
		}

		if err != nil {
			if isVerbose {
				fmt.Fprintf(c.App.Writer, "drand: error checking id %s: %s\n", address, err)
			} else {
				fmt.Fprintf(c.App.Writer, "drand: error checking id %s\n", address)
			}
			allGood = false
			invalidIds = append(invalidIds, address)
			continue
		}
		fmt.Fprintf(c.App.Writer, "drand: id %s answers correctly\n", address)
	}
	if !allGood {
		return fmt.Errorf("following nodes don't answer: %s", strings.Join(invalidIds, ","))
	}
	return nil
}

func checkIdentityAddress(lg log.Logger, conf *core.Config, addr string, tls bool, beaconID string) error {
	peer := net.CreatePeer(addr, tls)
	client := net.NewGrpcClientFromCertManager(lg, conf.Certs())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	metadata := &common2.Metadata{BeaconID: beaconID}
	identityResp, err := client.GetIdentity(ctx, peer, &drand.IdentityRequest{Metadata: metadata})
	if err != nil {
		return err
	}

	identity := &drand.Identity{
		Signature: identityResp.Signature,
		Tls:       identityResp.Tls,
		Address:   identityResp.Address,
		Key:       identityResp.Key,
	}
	sch, err := crypto.SchemeFromName(identityResp.SchemeName)
	if err != nil {
		lg.Errorw("received an invalid SchemeName in identity response", "received", identityResp.SchemeName)
		return err
	}
	id, err := key.IdentityFromProto(identity, sch)
	if err != nil {
		return err
	}
	if id.Address() != addr {
		return fmt.Errorf("mismatch of address: contact %s reply with %s", addr, id.Address())
	}
	return nil
}

// deleteBeaconCmd deletes all beacon in the database from the given round until
// the head of the chain
func deleteBeaconCmd(c *cli.Context, l log.Logger) error {
	conf := contextToConfig(c, l)
	ctx := c.Context

	startRoundStr := c.Args().First()
	sr, err := strconv.Atoi(startRoundStr)
	if err != nil {
		return fmt.Errorf("given round not valid: %d", sr)
	}

	startRound := uint64(sr)

	stores, err := getDBStoresPaths(c, l)
	if err != nil {
		return err
	}

	verbose := isVerbose(c)

	sch, err := crypto.GetSchemeFromEnv()
	if err != nil {
		return err
	}
	if sch.Name == crypto.DefaultSchemeID {
		ctx = chain.SetPreviousRequiredOnContext(ctx)
	}

	var er error
	for beaconID, storePath := range stores {
		if er != nil {
			return er
		}
		// Using an anonymous function to not leak the defer
		er = func() error {
			store, err := boltdb.NewBoltStore(ctx, l, path.Join(storePath, core.DefaultDBFolder), conf.BoltOptions())
			if err != nil {
				return fmt.Errorf("beacon id [%s] - invalid bolt store creation: %w", beaconID, err)
			}
			defer store.Close()

			lastBeacon, err := store.Last(ctx)
			if err != nil {
				return fmt.Errorf("beacon id [%s] - can't fetch last beacon: %w", beaconID, err)
			}
			if startRound > lastBeacon.Round {
				return fmt.Errorf("beacon id [%s] - given round is ahead of the chain: %d", beaconID, lastBeacon.Round)
			}
			if verbose {
				fmt.Printf("beacon id [%s] -  planning to delete %d beacons \n", beaconID, lastBeacon.Round-startRound)
			}

			for round := startRound; round <= lastBeacon.Round; round++ {
				err := store.Del(ctx, round)
				if err != nil {
					return fmt.Errorf("beacon id [%s] - error deleting round %d: %w", beaconID, round, err)
				}
				if verbose {
					fmt.Printf("beacon id [%s] - deleted beacon round %d \n", beaconID, round)
				}
			}
			return nil
		}()
	}

	return err
}

func isVerbose(c *cli.Context) bool {
	return c.IsSet(verboseFlag.Name)
}

func logLevel(c *cli.Context) int {
	if isVerbose(c) {
		return log.DebugLevel
	}

	return log.ErrorLevel
}

func logJSON(c *cli.Context) bool {
	return c.Bool(jsonFlag.Name)
}

func toArray(flags ...cli.Flag) []cli.Flag {
	return flags
}

func getGroup(c *cli.Context) (*key.Group, error) {
	g := &key.Group{}
	groupPath := c.Args().First()
	if err := testEmptyGroup(groupPath); err != nil {
		return nil, err
	}
	if err := key.Load(groupPath, g); err != nil {
		return nil, fmt.Errorf("drand: error loading group file: %w", err)
	}
	return g, nil
}

func checkArgs(c *cli.Context) error {
	if c.Bool(insecureFlag.Name) {
		if c.IsSet("tls-cert") || c.IsSet("tls-key") {
			return fmt.Errorf("option 'tls-disable' used with 'tls-cert' or 'tls-key': combination is not valid")
		}
	}
	if c.IsSet("certs-dir") {
		_, err := fs.Files(c.String("certs-dir"))
		if err != nil {
			return err
		}
	}

	return nil
}

func contextToConfig(c *cli.Context, l log.Logger) *core.Config {
	var opts []core.ConfigOption
	version := common3.GetAppVersion()

	if c.IsSet(pubListenFlag.Name) {
		opts = append(opts, core.WithPublicListenAddress(c.String(pubListenFlag.Name)))
	}
	if c.IsSet(privListenFlag.Name) {
		opts = append(opts, core.WithPrivateListenAddress(c.String(privListenFlag.Name)))
	}

	port := c.String(controlFlag.Name)
	if port != "" {
		opts = append(opts, core.WithControlPort(port))
	}
	if c.IsSet(folderFlag.Name) {
		opts = append(opts, core.WithConfigFolder(c.String(folderFlag.Name)))
	}
	opts = append(opts, core.WithVersion(fmt.Sprintf("drand/%s (%s)", version, gitCommit)))

	if c.Bool(insecureFlag.Name) {
		opts = append(opts, core.WithInsecure())
	} else {
		certPath, keyPath := c.String("tls-cert"), c.String("tls-key")
		opts = append(opts, core.WithTLS(certPath, keyPath))
	}
	if c.IsSet("certs-dir") {
		paths, err := fs.Files(c.String("certs-dir"))
		if err != nil {
			// it wouldn't reach here, as it was verified on checkArgs func before
			panic(err)
		}
		opts = append(opts, core.WithTrustedCerts(paths...))
	}

	if c.IsSet(tracesFlag.Name) {
		opts = append(opts, core.WithTracesEndpoint(c.String(tracesFlag.Name)))
	}

	if c.IsSet(tracesProbabilityFlag.Name) {
		opts = append(opts, core.WithTracesProbability(c.Float64(tracesProbabilityFlag.Name)))
	} else {
		//nolint:gomnd // Reset the trace probability to 5%
		opts = append(opts, core.WithTracesProbability(0.05))
	}

	switch chain.StorageType(c.String(storageTypeFlag.Name)) {
	case chain.BoltDB:
		opts = append(opts, core.WithDBStorageEngine(chain.BoltDB))
	case chain.PostgreSQL:
		opts = append(opts, core.WithDBStorageEngine(chain.PostgreSQL))

		if c.IsSet(pgDSNFlag.Name) {
			pgdsn := c.String(pgDSNFlag.Name)
			opts = append(opts, core.WithPgDSN(pgdsn))
		}
	case chain.MemDB:
		opts = append(opts,
			core.WithDBStorageEngine(chain.MemDB),
			core.WithMemDBSize(c.Int(memDBSizeFlag.Name)),
		)
	default:
		opts = append(opts, core.WithDBStorageEngine(chain.BoltDB))
	}

	conf := core.NewConfig(l, opts...)
	return conf
}

func getNodes(c *cli.Context) ([]*key.Node, error) {
	group, err := getGroup(c)
	if err != nil {
		return nil, err
	}
	var ids []*key.Node
	gids := group.Nodes
	if c.IsSet("nodes") {
		// search nodes listed on the flag in the group
		for _, addr := range strings.Split(c.String("nodes"), ",") {
			for _, gid := range gids {
				if gid.Addr == addr {
					ids = append(ids, gid)
				}
			}
		}
		if len(ids) == 0 {
			return nil, errors.New("addresses specified don't exist in group.toml")
		}
	} else {
		// select them all in order
		ids = gids
	}
	if len(ids) == 0 {
		return nil, errors.New("no nodes specified with --nodes are in the group file")
	}
	return ids, nil
}

func testEmptyGroup(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("can't open group path: %w", err)
	}
	defer file.Close()
	fi, err := file.Stat()
	if err != nil {
		return fmt.Errorf("can't open file info: %w", err)
	}
	if fi.Size() == 0 {
		return errors.New("group file empty")
	}
	return nil
}

func getBeaconID(c *cli.Context) string {
	return common3.GetCanonicalBeaconID(c.String(beaconIDFlag.Name))
}

func getDBStoresPaths(c *cli.Context, l log.Logger) (map[string]string, error) {
	conf := contextToConfig(c, l)
	stores := make(map[string]string)

	if c.IsSet(allBeaconsFlag.Name) {
		fi, err := os.ReadDir(conf.ConfigFolderMB())
		if err != nil {
			return nil, fmt.Errorf("error trying to read stores from config folder: %w", err)
		}
		for _, f := range fi {
			if f.IsDir() {
				stores[f.Name()] = path.Join(conf.ConfigFolderMB(), f.Name())
			}
		}
	} else {
		beaconID := getBeaconID(c)

		isPresent, err := fs.Exists(path.Join(conf.ConfigFolderMB(), beaconID))
		if err != nil || !isPresent {
			return nil, fmt.Errorf("beacon id [%s] - error trying to read store: %w", beaconID, err)
		}

		stores[beaconID] = path.Join(conf.ConfigFolderMB(), beaconID)
	}

	return stores, nil
}

func getKeyStores(c *cli.Context, l log.Logger) (map[string]key.Store, error) {
	conf := contextToConfig(c, l)

	if c.IsSet(allBeaconsFlag.Name) {
		return key.NewFileStores(conf.ConfigFolderMB())
	}

	beaconID := getBeaconID(c)

	store := key.NewFileStore(conf.ConfigFolderMB(), beaconID)
	stores := map[string]key.Store{beaconID: store}

	return stores, nil
}

func deprecatedShareCommand(_ *cli.Context) error {
	return errors.New("the share command has been removed! Please use `drand dkg` instead")
}
