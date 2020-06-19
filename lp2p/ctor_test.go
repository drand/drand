package lp2p

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/drand/drand/log"
)

func TestCreateThenLoadPrivKey(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}

	// should not exist yet...
	identityPath := path.Join(dir, "identify.key")

	priv0, err := LoadOrCreatePrivKey(identityPath, log.DefaultLogger())
	if err != nil {
		t.Fatal(err)
	}

	// read again, should be the same
	priv1, err := LoadOrCreatePrivKey(identityPath, log.DefaultLogger())
	if err != nil {
		t.Fatal(err)
	}

	if !priv0.Equals(priv1) {
		t.Fatal(fmt.Errorf("private key not persisted and/or not read back properly"))
	}
}

func TestCreatePrivKeyMkdirp(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}

	// should not exist yet and has an intermediate dir that does not exist
	identityPath := path.Join(dir, "not-exists-dir", "identify.key")

	priv0, err := LoadOrCreatePrivKey(identityPath, log.DefaultLogger())
	if err != nil {
		t.Fatal(err)
	}

	// read again, should be the same
	priv1, err := LoadOrCreatePrivKey(identityPath, log.DefaultLogger())
	if err != nil {
		t.Fatal(err)
	}

	if !priv0.Equals(priv1) {
		t.Fatal(fmt.Errorf("private key not persisted and/or not read back properly"))
	}
}
