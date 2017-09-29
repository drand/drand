package protobuf

import (
	"encoding/hex"
	"fmt"
)

// Go-based protobuf definition for the example Person message format
type Person struct {
	Name  string        // = 1, required
	Id    int32         // = 2, required
	Email *string       // = 3, optional
	Phone []PhoneNumber // = 4, repeated
}

type PhoneType uint32 // protobuf enums are uint32
const (
	MOBILE PhoneType = iota // = 0
	HOME                    // = 1
	WORK                    // = 2
)

type PhoneNumber struct {
	Number string     // = 1, required
	Type   *PhoneType // = 2, optional
}

// This example defines, encodes, and decodes a Person message format
// equivalent to the example used in the Protocol Buffers overview.
func Example_protobuf() {

	// Create a Person record
	email := "alice@somewhere"
	ptype := WORK
	person := Person{"Alice", 123, &email,
		[]PhoneNumber{PhoneNumber{"111-222-3333", nil},
			PhoneNumber{"444-555-6666", &ptype}}}

	// Encode it
	buf, err := Encode(&person)
	if err != nil {
		panic("Encode failed: " + err.Error())
	}
	fmt.Print(hex.Dump(buf))

	// Decode it
	person2 := Person{}
	if err := Decode(buf, &person2); err != nil {
		panic("Decode failed")
	}

	// Output:
	// 00000000  0a 05 41 6c 69 63 65 10  f6 01 1a 0f 61 6c 69 63  |..Alice.....alic|
	// 00000010  65 40 73 6f 6d 65 77 68  65 72 65 22 0e 0a 0c 31  |e@somewhere"...1|
	// 00000020  31 31 2d 32 32 32 2d 33  33 33 33 22 10 0a 0c 34  |11-222-3333"...4|
	// 00000030  34 34 2d 35 35 35 2d 36  36 36 36 10 02           |44-555-6666..|
}
