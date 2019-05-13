/*
* This file is used to generate the same output as a drand node would do
* The chosen message is formated and signed with a newly generated public key at
* each round
* The message, the signature and the public key are then put into the files
* info/ and public under the fields previous, randomness and pub_key
* respectively
 */

package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"

	"go.dedis.ch/kyber/v3/pairing/bn256"
	"go.dedis.ch/kyber/v3/sign/bls"
	"go.dedis.ch/kyber/v3/util/random"
	//"go.dedis.ch/kyber/v3/share/dkg/pedersen"
)

//Message formats prev and round to creates the signed content
func Message(msg []byte, round uint64) []byte {
	var buff bytes.Buffer
	binary.Write(&buff, binary.BigEndian, round)
	buff.Write(msg)
	return buff.Bytes()
}

func main() {
	suite := bn256.NewSuite()
	//msg
	round := uint64(1)
	tmp := []byte("func very_random_function() { return 4 }")
	msg := Message(tmp, round)
	println("previous:")
	println(hex.EncodeToString(msg))
	//pub_key
	private, public := bls.NewKeyPair(suite, random.New())
	println("pub_key:")
	p, _ := public.MarshalBinary()
	println(hex.EncodeToString(p))
	//sig
	sig, _ := bls.Sign(suite, private, msg)
	println("randomness:")
	println(hex.EncodeToString(sig))
	//ver
	if bls.Verify(suite, public, msg, sig) == nil {
		println("verified")
	}
}
