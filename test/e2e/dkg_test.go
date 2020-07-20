package e2e

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"
)

func tempDir() string {
	name, err := ioutil.TempDir(os.TempDir(), "drand-e2e-test")
	if err != nil {
		panic(err)
	}
	return path.Join(os.TempDir(), name)
}

func TestDKG(t *testing.T) {
	f0 := tempDir()
	f1 := tempDir()
	f2 := tempDir()

	t0 := NewTerminal()
	t1 := NewTerminal()
	t2 := NewTerminal()

	a0 := "127.0.0.1:3000"
	a1 := "127.0.0.1:3100"
	a2 := "127.0.0.1:3200"

	t0.Run(fmt.Sprintf("drand generate-keypair --tls-disable --folder %s %s", f0, a0))
	t1.Run(fmt.Sprintf("drand generate-keypair --tls-disable --folder %s %s", f1, a1))
	t2.Run(fmt.Sprintf("drand generate-keypair --tls-disable --folder %s %s", f2, a2))

	t0.Wait(time.Minute)
	t1.Wait(time.Minute)
	t2.Wait(time.Minute)
}
