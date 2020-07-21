package e2e

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/drand/drand/test/e2e/commander"
	"github.com/kabukky/httpscerts"
)

const (
	host         = "127.0.0.1"
	certFilename = "server.crt"
	keyFilename  = "server.key"
	secret       = "_DRANDO_SECRET_IS_32_CHARACTERS_MINIMUM"
)

func tempDir(t *testing.T) string {
	name, err := ioutil.TempDir(os.TempDir(), "drand-e2e-test")
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func generateCerts(t *testing.T, paths ...string) {
	for _, p := range paths {
		err := httpscerts.Generate(path.Join(p, certFilename), path.Join(p, keyFilename), host)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// trustedCertsDir links cert files from the passed paths into a new temporary
// directory that can be used as the value for --certs-dir.
func trustedCertsDir(t *testing.T, paths ...string) string {
	dir := tempDir(t)
	for i, p := range paths {
		err := os.Link(path.Join(p, certFilename), path.Join(dir, fmt.Sprintf("%d.crt", i)))
		if err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestDKG(t *testing.T) {
	alphaDir, bravoDir, charlieDir := tempDir(t), tempDir(t), tempDir(t)
	alphaPrivPort, bravoPrivPort, charliePrivPort := "3000", "3100", "3200"
	alphaCtlPort, bravoCtlPort, charlieCtlPort := "3001", "3101", "3201"
	alphaPubPort, bravoPubPort, charliePubPort := "3002", "3102", "3202"

	alphaFolder := path.Join(alphaDir, ".drand")
	bravoFolder := path.Join(bravoDir, ".drand")
	charlieFolder := path.Join(charlieDir, ".drand")

	alphaTerm0 := commander.NewTerminal("alpha")
	bravoTerm0 := commander.NewTerminal("bravo")
	charlieTerm0 := commander.NewTerminal("charlie")

	alphaTerm0.Run(t, "drand", "generate-keypair", "--folder", alphaFolder, host+":"+alphaPrivPort)
	alphaTerm0.AwaitOutput(t, "Generated keys")
	alphaTerm0.AwaitSuccess(t)

	bravoTerm0.Run(t, "drand", "generate-keypair", "--folder", bravoFolder, host+":"+bravoPrivPort)
	bravoTerm0.AwaitOutput(t, "Generated keys")
	bravoTerm0.AwaitSuccess(t)

	charlieTerm0.Run(t, "drand", "generate-keypair", "--folder", charlieFolder, host+":"+charliePrivPort)
	charlieTerm0.AwaitOutput(t, "Generated keys")
	charlieTerm0.AwaitSuccess(t)

	generateCerts(t, alphaDir, bravoDir, charlieDir)
	certsDir := trustedCertsDir(t, alphaDir, bravoDir, charlieDir)

	alphaTerm1 := commander.NewTerminal("alpha daemon")
	alphaTerm1.Run(t,
		"drand", "start",
		"--tls-cert", path.Join(alphaDir, certFilename),
		"--tls-key", path.Join(alphaDir, keyFilename),
		"--certs-dir", certsDir,
		"--folder", alphaFolder,
		"--private-listen", host+":"+alphaPrivPort,
		"--control", alphaCtlPort,
		"--public-listen", host+":"+alphaPubPort,
	)
	alphaTerm1.AwaitOutput(t, "expect to run DKG")

	bravoTerm1 := commander.NewTerminal("bravo daemon")
	bravoTerm1.Run(t,
		"drand", "start",
		"--tls-cert", path.Join(bravoDir, certFilename),
		"--tls-key", path.Join(bravoDir, keyFilename),
		"--certs-dir", certsDir,
		"--folder", bravoFolder,
		"--private-listen", host+":"+bravoPrivPort,
		"--control", bravoCtlPort,
		"--public-listen", host+":"+bravoPubPort,
	)
	bravoTerm1.AwaitOutput(t, "expect to run DKG")

	charlieTerm1 := commander.NewTerminal("charlie daemon")
	charlieTerm1.Run(t,
		"drand", "start",
		"--tls-cert", path.Join(charlieDir, certFilename),
		"--tls-key", path.Join(charlieDir, keyFilename),
		"--certs-dir", certsDir,
		"--folder", charlieFolder,
		"--private-listen", host+":"+charliePrivPort,
		"--control", charlieCtlPort,
		"--public-listen", host+":"+charliePubPort,
	)
	charlieTerm1.AwaitOutput(t, "expect to run DKG")

	alphaTerm2 := commander.NewTerminal("alpha share leader")
	alphaTerm2.Run(t,
		"drand", "share",
		"--control", alphaCtlPort,
		"--leader",
		"--nodes", "3",
		"--threshold", "2",
		"--secret", secret,
		"--period", "10s",
	)
	alphaTerm2.AwaitOutput(t, "Initiating the DKG as a leader")

	bravoTerm2 := commander.NewTerminal("bravo share participant")
	bravoTerm2.Run(t,
		"drand", "share",
		"--control", bravoCtlPort,
		"--connect", host+":"+alphaPrivPort,
		"--secret", secret,
	)
	bravoTerm2.AwaitOutput(t, "Participating to the setup of the DKG")

	charlieTerm2 := commander.NewTerminal("charlie share participant")
	charlieTerm2.Run(t,
		"drand", "share",
		"--control", charlieCtlPort,
		"--connect", host+":"+alphaPrivPort,
		"--secret", secret,
	)
	charlieTerm2.AwaitOutput(t, "Participating to the setup of the DKG")

	alphaTerm1.AwaitSuccess(t)
}
