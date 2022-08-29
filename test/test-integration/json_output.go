package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/drand/drand/chain"
	dscheme "github.com/drand/drand/common/scheme"
	"github.com/drand/drand/key"
	"github.com/drand/kyber/sign/bls"
	"github.com/drand/kyber/util/random"
)

// This binary returns a "fake" JSON output that is used as a reference for
// interoperability testing with the other repos such as drandjs.

func main() {
	private := key.KeyGroup.Scalar().Pick(random.New())
	public := key.KeyGroup.Point().Mul(private, nil)
	scheme := bls.NewSchemeOnG2(key.Pairing)
	round := 1984

	previousSig, err := scheme.Sign(private, []byte("Test Signature"))
	if err != nil {
		panic(err)
	}

	sch := dscheme.GetSchemeFromEnv()
	verifier := chain.NewVerifier(sch)

	msg := verifier.DigestMessage(uint64(round), previousSig)
	signature, err := scheme.Sign(private, msg)
	if err != nil {
		panic(err)
	}

	pub, _ := public.MarshalBinary()
	type Export struct {
		Signature string
		Round     int
		Previous  string
		Public    string
	}
	ex := &Export{
		Signature: hex.EncodeToString(signature),
		Round:     round,
		Previous:  hex.EncodeToString(previousSig),
		Public:    hex.EncodeToString(pub),
	}
	out, _ := json.MarshalIndent(ex, "", "    ")
	fmt.Println(string(out))
}
