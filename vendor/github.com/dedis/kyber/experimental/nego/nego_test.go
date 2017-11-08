// +build experimental

package nego

import (
	"fmt"
	"testing"

	"github.com/dedis/kyber/abstract"
	"github.com/dedis/kyber/edwards"
	"github.com/dedis/kyber/random"
)

// Simple harness to create lots of fake ciphersuites out of a few real ones,
// for testing purposes.
type fakeSuite struct {
	abstract.Suite
	idx int
}

func (f *fakeSuite) String() string {
	return fmt.Sprintf("%s(%d)", f.Suite.String(), f.idx)
}

func TestNego(t *testing.T) {

	realSuites := []abstract.Suite{
		edwards.NewAES128SHA256Ed25519(true),
	}

	fakery := 10
	nentries := 10
	datalen := 16

	suites := make([]abstract.Suite, 0)
	for i := range realSuites {
		real := realSuites[i]
		for j := 0; j < fakery; j++ {
			suites = append(suites, &fakeSuite{real, j})
		}
	}

	nlevels := 5
	suiteLevel := make(map[abstract.Suite]int)
	entries := make([]Entry, 0)
	for i := range suites {
		suiteLevel[suites[i]] = nlevels
		nlevels++ // vary it a bit for testing

		// Create some entrypoints with this suite
		s := suites[i]
		for j := 0; j < nentries; j++ {
			pri := s.Scalar().Pick(random.Stream)
			pub := s.Point().Mul(nil, pri)
			data := make([]byte, datalen)
			entries = append(entries, Entry{s, pub, data})
		}
	}

	w := Writer{}
	_, err := w.Layout(suiteLevel, entries, nil)
	if err != nil {
		t.Log(err)
		t.FailNow()
	}
}
