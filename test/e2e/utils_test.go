package e2e

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/drand/drand/test"
	"github.com/kabukky/httpscerts"
)

const (
	host         = "127.0.0.1"
	certFilename = "server.crt"
	keyFilename  = "server.key"
)

func tempDir(t *testing.T) string {
	name, err := ioutil.TempDir(os.TempDir(), "drand-e2e-test")
	if err != nil {
		t.Fatal(err)
	}
	return name
}

// trustedCertsDir links cert files from the passed paths into a new temporary
// directory that can be used as the value for --certs-dir.
func trustedCertsDir(t *testing.T, paths ...string) string {
	dir := tempDir(t)
	for i, p := range paths {
		err := os.Link(p, path.Join(dir, fmt.Sprintf("%d.crt", i)))
		if err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

type Config struct {
	basedir string
	folder  string
	ports   Ports
	tls     TLS
}

type TLS struct {
	certpath string
	keypath  string
}

type Ports struct {
	priv string
	ctl  string
	pub  string
}

func generateConfigs(t *testing.T, n int) []Config {
	var configs []Config
	for i := 0; i < n; i++ {
		basedir := tempDir(t)
		certpath := path.Join(basedir, certFilename)
		keypath := path.Join(basedir, keyFilename)
		if err := httpscerts.Generate(certpath, keypath, host); err != nil {
			t.Fatal(err)
		}
		c := Config{
			basedir: basedir,
			folder:  path.Join(basedir, ".drand"),
			ports:   Ports{test.FreePort(), test.FreePort(), test.FreePort()},
			tls:     TLS{certpath, keypath},
		}
		configs = append(configs, c)
	}

	return configs
}
