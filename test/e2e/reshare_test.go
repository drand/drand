package e2e

import (
	"path"
	"testing"

	"github.com/drand/drand/test/e2e/commander/terminal"
	"github.com/drand/drand/test/e2e/commander/terminal/manager"
	"github.com/google/uuid"
)

func TestReshareAddParticipant(t *testing.T) {
	confs := generateConfigs(t, 3)
	alphaConf, bravoConf, charlieConf := confs[0], confs[1], confs[2]
	secret := uuid.New().String()

	alphaTerm0 := terminal.ForTesting("alpha")
	bravoTerm0 := terminal.ForTesting("bravo")

	alphaTerm0.Run(t, "drand", generateKeypairArgs(alphaConf)...)
	bravoTerm0.Run(t, "drand", generateKeypairArgs(bravoConf)...)

	keyMgr := manager.ForTesting(alphaTerm0, bravoTerm0)
	keyMgr.AwaitOutput(t, "Generated keys")
	keyMgr.AwaitSuccess(t)

	certsDir := trustedCertsDir(t, alphaConf.tls.certpath, bravoConf.tls.certpath, charlieConf.tls.certpath)

	alphaTerm0.Run(t, "drand", startWithTLSArgs(alphaConf, certsDir)...)
	bravoTerm0.Run(t, "drand", startWithTLSArgs(bravoConf, certsDir)...)

	daemonMgr := manager.ForTesting(alphaTerm0, bravoTerm0)
	daemonMgr.AwaitOutput(t, "expect to run DKG")
	defer daemonMgr.Kill(t)

	alphaTerm1 := terminal.ForTesting("alpha share")
	alphaTerm1.Run(t,
		"drand", "share",
		"--control", alphaConf.ports.ctl,
		"--leader",
		"--nodes", "2",
		"--threshold", "2",
		"--secret", secret,
		"--period", "2s",
		"--timeout", "2s",
	)
	alphaTerm1.AwaitOutput(t, "Initiating the DKG as a leader")

	bravoTerm1 := terminal.ForTesting("bravo share")
	bravoTerm1.Run(t, "drand", shareParticipantArgs(bravoConf, alphaConf, secret)...)

	// wait for share processes to cleanly exit
	shareMgr := manager.ForTesting(alphaTerm1, bravoTerm1)
	shareMgr.AwaitSuccess(t)

	// wait for beacon generation
	daemonMgr.AwaitOutput(t, "NEW_BEACON_STORED=\"{ round: 2")

	// DKG is now complete, we can begin a reshare and bring charlie into the fold
	charlieTerm0 := terminal.ForTesting("charlie")
	charlieTerm0.Run(t, "drand", generateKeypairArgs(charlieConf)...)
	charlieTerm0.AwaitSuccess(t)
	charlieTerm0.Run(t, "drand", startWithTLSArgs(charlieConf, certsDir)...)
	defer charlieTerm0.Kill(t)
	charlieTerm0.AwaitOutput(t, "expect to run DKG")

	secret = uuid.New().String() // new secret for the reshare

	// bravo will be the leader this time
	bravoTerm1.Run(t,
		"drand", "share",
		"--control", bravoConf.ports.ctl,
		"--leader",
		"--transition", // signals reshare
		"--nodes", "3",
		"--threshold", "2",
		"--secret", secret,
		"--period", "2s",
		"--timeout", "2s",
	)
	bravoTerm1.AwaitOutput(t, "Initiating the resharing as a leader")

	alphaTerm1.Run(t,
		"drand", "share",
		"--control", alphaConf.ports.ctl,
		"--connect", host+":"+bravoConf.ports.priv,
		"--secret", secret,
		"--transition",
	)
	alphaTerm1.AwaitOutput(t, "Participating to the resharing")

	charlieTerm1 := terminal.ForTesting("charlie share")
	charlieTerm1.Run(t,
		"drand", "share",
		"--control", charlieConf.ports.ctl,
		"--connect", host+":"+bravoConf.ports.priv,
		"--secret", secret,
		"--from", path.Join(alphaConf.folder, "groups", "drand_group.toml"),
	)
	charlieTerm1.AwaitOutput(t, "Participating to the resharing")
	charlieTerm1.AwaitSuccess(t)

	// charlie should now be storing beacons
	charlieTerm0.AwaitOutput(t, "NEW_BEACON_STORED")
}

func TestReshareRemoveParticipant(t *testing.T) {
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

	alphaTerm0.Run(t, "drand", startWithTLSArgs(alphaConf, certsDir)...)
	bravoTerm0.Run(t, "drand", startWithTLSArgs(bravoConf, certsDir)...)
	charlieTerm0.Run(t, "drand", startWithTLSArgs(charlieConf, certsDir)...)

	daemonMgr := manager.ForTesting(alphaTerm0, bravoTerm0, charlieTerm0)
	daemonMgr.AwaitOutput(t, "expect to run DKG")
	defer daemonMgr.Kill(t)

	alphaTerm1 := terminal.ForTesting("alpha share")
	alphaTerm1.Run(t,
		"drand", "share",
		"--control", alphaConf.ports.ctl,
		"--leader",
		"--nodes", "3",
		"--threshold", "2",
		"--secret", secret,
		"--period", "2s",
		"--timeout", "2s",
	)
	alphaTerm1.AwaitOutput(t, "Initiating the DKG as a leader")

	bravoTerm1 := terminal.ForTesting("bravo share")
	bravoTerm1.Run(t, "drand", shareParticipantArgs(bravoConf, alphaConf, secret)...)

	charlieTerm1 := terminal.ForTesting("charlie share")
	charlieTerm1.Run(t, "drand", shareParticipantArgs(charlieConf, alphaConf, secret)...)

	// wait for share processes to cleanly exit
	shareMgr := manager.ForTesting(alphaTerm1, bravoTerm1, charlieTerm1)
	shareMgr.AwaitSuccess(t)

	// wait for beacon generation
	daemonMgr.AwaitOutput(t, "NEW_BEACON_STORED=\"{ round: 2")

	// DKG is now complete, we can begin a reshare and remove alpha
	secret = uuid.New().String() // new secret for the reshare

	// bravo will be the leader this time
	bravoTerm1.Run(t,
		"drand", "share",
		"--control", bravoConf.ports.ctl,
		"--leader",
		"--transition", // signals reshare
		"--nodes", "2",
		"--threshold", "2",
		"--secret", secret,
		"--period", "2s",
		"--timeout", "2s",
	)
	bravoTerm1.AwaitOutput(t, "Initiating the resharing as a leader")

	charlieTerm1.Run(t,
		"drand", "share",
		"--control", charlieConf.ports.ctl,
		"--connect", host+":"+bravoConf.ports.priv,
		"--secret", secret,
		"--transition",
	)
	charlieTerm1.AwaitOutput(t, "Participating to the resharing")

	// share processes should now exit cleanly
	manager.ForTesting(bravoTerm1, charlieTerm1).AwaitSuccess(t)

	// beacon generation should resume on bravo and charlie
	manager.ForTesting(bravoTerm0, charlieTerm0).AwaitOutput(t, "NEW_BEACON_STORED")
}
