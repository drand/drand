package e2e

import (
	"testing"

	"github.com/drand/drand/test/e2e/commander/terminal"
	"github.com/drand/drand/test/e2e/commander/terminal/manager"
	"github.com/google/uuid"
)

func TestDKG(t *testing.T) {
	confs := generateConfigs(t, 3)
	alphaConf, bravoConf, charlieConf := confs[0], confs[1], confs[2]
	secret := uuid.New().String()

	alphaTerm0 := terminal.ForTesting("alpha")
	bravoTerm0 := terminal.ForTesting("bravo")
	charlieTerm0 := terminal.ForTesting("charlie")

	alphaTerm0.Run(t, "drand", generateKeypairArgs(alphaConf)...)
	bravoTerm0.Run(t, "drand", generateKeypairArgs(bravoConf)...)
	charlieTerm0.Run(t, "drand", generateKeypairArgs(charlieConf)...)

	keyMgr := manager.ForTesting(alphaTerm0, bravoTerm0, charlieTerm0)
	keyMgr.AwaitOutput(t, "Generated keys")
	keyMgr.AwaitSuccess(t)

	certsDir := trustedCertsDir(t, alphaConf.tls.certpath, bravoConf.tls.certpath, charlieConf.tls.certpath)

	alphaTerm1 := terminal.ForTesting("alpha daemon")
	alphaTerm1.Run(t, "drand", startWithTLSArgs(alphaConf, certsDir)...)

	bravoTerm1 := terminal.ForTesting("bravo daemon")
	bravoTerm1.Run(t, "drand", startWithTLSArgs(bravoConf, certsDir)...)

	charlieTerm1 := terminal.ForTesting("charlie daemon")
	charlieTerm1.Run(t, "drand", startWithTLSArgs(charlieConf, certsDir)...)

	daemonMgr := manager.ForTesting(alphaTerm1, bravoTerm1, charlieTerm1)
	daemonMgr.AwaitOutput(t, "expect to run DKG")
	defer daemonMgr.Kill(t)

	alphaTerm2 := terminal.ForTesting("alpha share leader")
	alphaTerm2.Run(t,
		"drand", "share",
		"--control", alphaConf.ports.ctl,
		"--leader",
		"--nodes", "3",
		"--threshold", "2",
		"--secret", secret,
		"--period", "2s",
		"--timeout", "2s",
	)
	alphaTerm2.AwaitOutput(t, "Initiating the DKG as a leader")

	bravoTerm2 := terminal.ForTesting("bravo share participant")
	bravoTerm2.Run(t, "drand", shareParticipantArgs(bravoConf, alphaConf, secret)...)
	bravoTerm2.AwaitOutput(t, "Participating to the setup of the DKG")

	charlieTerm2 := terminal.ForTesting("charlie share participant")
	charlieTerm2.Run(t, "drand", shareParticipantArgs(charlieConf, alphaConf, secret)...)
	charlieTerm2.AwaitOutput(t, "Participating to the setup of the DKG")

	// wait for share processes to cleanly exit
	shareMgr := manager.ForTesting(alphaTerm2, bravoTerm2, charlieTerm2)
	shareMgr.AwaitSuccess(t)

	// wait for beacon generation
	daemonMgr.AwaitOutput(t, "NEW_BEACON_STORED=\"{ round: 2")
}

func TestDKGNoTLS(t *testing.T) {
	confs := generateConfigs(t, 3)
	alphaConf, bravoConf, charlieConf := confs[0], confs[1], confs[2]
	secret := uuid.New().String()

	alphaTerm0 := terminal.ForTesting("alpha")
	bravoTerm0 := terminal.ForTesting("bravo")
	charlieTerm0 := terminal.ForTesting("charlie")

	alphaTerm0.Run(t, "drand", generateKeypairArgs(alphaConf)...)
	bravoTerm0.Run(t, "drand", generateKeypairArgs(bravoConf)...)
	charlieTerm0.Run(t, "drand", generateKeypairArgs(charlieConf)...)

	keyMgr := manager.ForTesting(alphaTerm0, bravoTerm0, charlieTerm0)
	keyMgr.AwaitOutput(t, "Generated keys")
	keyMgr.AwaitSuccess(t)

	alphaTerm1 := terminal.ForTesting("alpha daemon")
	alphaTerm1.Run(t,
		"drand", "start",
		"--tls-disable",
		"--folder", alphaConf.folder,
		"--private-listen", host+":"+alphaConf.ports.priv,
		"--control", alphaConf.ports.ctl,
		"--public-listen", host+":"+alphaConf.ports.pub,
	)

	bravoTerm1 := terminal.ForTesting("bravo daemon")
	bravoTerm1.Run(t,
		"drand", "start",
		"--tls-disable",
		"--folder", bravoConf.folder,
		"--private-listen", host+":"+bravoConf.ports.priv,
		"--control", bravoConf.ports.ctl,
		"--public-listen", host+":"+bravoConf.ports.pub,
	)

	charlieTerm1 := terminal.ForTesting("charlie daemon")
	charlieTerm1.Run(t,
		"drand", "start",
		"--tls-disable",
		"--folder", charlieConf.folder,
		"--private-listen", host+":"+charlieConf.ports.priv,
		"--control", charlieConf.ports.ctl,
		"--public-listen", host+":"+charlieConf.ports.pub,
	)

	daemonMgr := manager.ForTesting(alphaTerm1, bravoTerm1, charlieTerm1)
	daemonMgr.AwaitOutput(t, "expect to run DKG")
	defer daemonMgr.Kill(t)

	alphaTerm2 := terminal.ForTesting("alpha share leader")
	alphaTerm2.Run(t,
		"drand", "share",
		"--tls-disable",
		"--control", alphaConf.ports.ctl,
		"--leader",
		"--nodes", "3",
		"--threshold", "2",
		"--secret", secret,
		"--period", "2s",
		"--timeout", "2s",
	)
	alphaTerm2.AwaitOutput(t, "Initiating the DKG as a leader")

	bravoTerm2 := terminal.ForTesting("bravo share participant")
	bravoTerm2.Run(t,
		"drand", "share",
		"--tls-disable",
		"--control", bravoConf.ports.ctl,
		"--connect", host+":"+alphaConf.ports.priv,
		"--secret", secret,
	)
	bravoTerm2.AwaitOutput(t, "Participating to the setup of the DKG")

	charlieTerm2 := terminal.ForTesting("charlie share participant")
	charlieTerm2.Run(t,
		"drand", "share",
		"--tls-disable",
		"--control", charlieConf.ports.ctl,
		"--connect", host+":"+alphaConf.ports.priv,
		"--secret", secret,
	)
	charlieTerm2.AwaitOutput(t, "Participating to the setup of the DKG")

	// wait for share processes to cleanly exit
	shareMgr := manager.ForTesting(alphaTerm2, bravoTerm2, charlieTerm2)
	shareMgr.AwaitOutput(t, "Hash of the group configuration")
	shareMgr.AwaitSuccess(t)

	// wait for beacon generation
	daemonMgr.AwaitOutput(t, "NEW_BEACON_STORED=\"{ round: 2")
}

// TODO: this test currently stops the leader share command after 1 participant
// has joined. Consider adding another test where the leader share command is
// killed before any participants join: https://github.com/drand/drand/issues/709
func TestDKGWithStoppedLeaderShareCommand(t *testing.T) {
	confs := generateConfigs(t, 3)
	alphaConf, bravoConf, charlieConf := confs[0], confs[1], confs[2]
	secret := uuid.New().String()

	alphaTerm0 := terminal.ForTesting("alpha")
	bravoTerm0 := terminal.ForTesting("bravo")
	charlieTerm0 := terminal.ForTesting("charlie")

	alphaTerm0.Run(t, "drand", generateKeypairArgs(alphaConf)...)
	bravoTerm0.Run(t, "drand", generateKeypairArgs(bravoConf)...)
	charlieTerm0.Run(t, "drand", generateKeypairArgs(charlieConf)...)

	keyMgr := manager.ForTesting(alphaTerm0, bravoTerm0, charlieTerm0)
	keyMgr.AwaitOutput(t, "Generated keys")
	keyMgr.AwaitSuccess(t)

	certsDir := trustedCertsDir(t, alphaConf.tls.certpath, bravoConf.tls.certpath, charlieConf.tls.certpath)

	alphaTerm1 := terminal.ForTesting("alpha daemon")
	alphaTerm1.Run(t, "drand", startWithTLSArgs(alphaConf, certsDir)...)

	bravoTerm1 := terminal.ForTesting("bravo daemon")
	bravoTerm1.Run(t, "drand", startWithTLSArgs(bravoConf, certsDir)...)

	charlieTerm1 := terminal.ForTesting("charlie daemon")
	charlieTerm1.Run(t, "drand", startWithTLSArgs(charlieConf, certsDir)...)

	daemonMgr := manager.ForTesting(alphaTerm1, bravoTerm1, charlieTerm1)
	daemonMgr.AwaitOutput(t, "expect to run DKG")
	defer daemonMgr.Kill(t)

	alphaTerm2 := terminal.ForTesting("alpha share leader")
	alphaTerm2.Run(t,
		"drand", "share",
		"--control", alphaConf.ports.ctl,
		"--leader",
		"--nodes", "3",
		"--threshold", "2",
		"--secret", secret,
		"--period", "2s",
		"--timeout", "2s",
	)
	alphaTerm2.AwaitOutput(t, "Initiating the DKG as a leader")

	bravoTerm2 := terminal.ForTesting("bravo share participant")
	bravoTerm2.Run(t, "drand", shareParticipantArgs(bravoConf, alphaConf, secret)...)
	bravoTerm2.AwaitOutput(t, "Participating to the setup of the DKG")

	// kill the leader share command before all participants join
	alphaTerm2.Kill(t)

	charlieTerm2 := terminal.ForTesting("charlie share participant")
	charlieTerm2.Run(t, "drand", shareParticipantArgs(charlieConf, alphaConf, secret)...)
	charlieTerm2.AwaitOutput(t, "Participating to the setup of the DKG")

	// wait for participant processes to cleanly exit
	participantMgr := manager.ForTesting(bravoTerm2, charlieTerm2)
	participantMgr.AwaitSuccess(t)

	// wait for beacon generation
	daemonMgr.AwaitOutput(t, "NEW_BEACON_STORED=\"{ round: 2")
}
