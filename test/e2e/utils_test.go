package e2e

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/kabukky/httpscerts"
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

func x3(f func() string) (string, string, string) {
	return f(), f(), f()
}
