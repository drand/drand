// +build experimental

package cipher

import (
	"reflect"
)

// Generic reflection-driven "universal constructor" interface,
// which determines how to create concrete objects
// instantiating a given set of kyber.interface types.
type Constructor interface {

	// Create a fresh object of a given (usually interface) type.
	New(t reflect.Type) interface{}
}
