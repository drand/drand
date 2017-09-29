package protobuf

import (
	"encoding/hex"
	"fmt"
)

type Test2 struct {
	_ interface{} // = 1
	B string      // = 2
}

// This example encodes the Test2 message illustrating strings
// in the Protocol Buffers encoding specification.
func ExampleEncode_test2() {

	t := Test2{B: "testing"}
	buf, _ := Encode(&t)
	fmt.Print(hex.Dump(buf))

	// Output:
	// 00000000  12 07 74 65 73 74 69 6e  67                       |..testing|
}
