// To compile use
// GOOS=js GOARCH=wasm go build -o main.wasm main.go
//
package main

import (
	"encoding/hex"
	"fmt"
	"syscall/js"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
)

var done = make(chan struct{})

func main() {
	callback := js.FuncOf(verifyBeacon)
	defer callback.Release()
	setResult := js.Global().Get("verifyBeacon")
	setResult.Invoke(callback)
	<-done
}

// verifyBeacon expects the arguments in order:
// 1. public key in hexadecimal
// 2. previous signature in hexadecimal
// 3. round in base 10
// 4. signature in hexadecimal
func verifyBeacon(value js.Value, args []js.Value) interface{} {
	defer func() { done <- struct{}{} }()
	if len(args) != 4 {
		return fmt.Errorf("drand-go: not enough arguments to verify beacon")
	}

	publicBuff, err := hex.DecodeString(args[0].String())
	if err != nil {
		return fmt.Errorf("drand-go: invalid hexadecimal for public key: %v", err)
	}

	prevBuff, err := hex.DecodeString(args[1].String())
	if err != nil {
		return fmt.Errorf("drand-go: invalid hexadecimal for previous signature: %v", err)
	}

	sigBuff, err := hex.DecodeString(args[3].String())
	if err != nil {
		return fmt.Errorf("drand-go: invalid hexadecimal for signature: %v", err)
	}

	round := args[2].Int()
	if int(uint64(round)) != round {
		return fmt.Errorf("drand-go: round is not valid %d", round)
	}

	pub := key.KeyGroup.Point()
	if err := pub.UnmarshalBinary(publicBuff); err != nil {
		return fmt.Errorf("public key invalid: %v", err)
	}
	return chain.Verify(pub, prevBuff, sigBuff, uint64(round))
}
