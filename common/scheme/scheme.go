package scheme

import (
	"fmt"
	"os"
)

// DefaultSchemeID is the default scheme ID.
const DefaultSchemeID = "pedersen-bls-chained"

// UnchainedSchemeID is the scheme id used to set unchained randomness on beacons.
const UnchainedSchemeID = "pedersen-bls-unchained"

// Scheme is used to group a set of configurations related to the scheme beacons will use to generate randomness
type Scheme struct {
	ID              string
	DecouplePrevSig bool
}

var schemes = []Scheme{{ID: DefaultSchemeID, DecouplePrevSig: false}, {ID: UnchainedSchemeID, DecouplePrevSig: true}}

// GetSchemeByID allows the user to retrieve the scheme configuration looking by its ID. It will return a boolean which indicates
// if the scheme was found or not.
func GetSchemeByID(id string) (scheme Scheme, found bool) {
	for _, t := range schemes {
		if t.ID == id {
			return t, true
		}
	}

	return Scheme{}, false
}

// GetSchemeByIDWithDefault allows the user to retrieve the scheme configuration looking by its ID. It will return a boolean which indicates
// if the scheme was foound or not. In addition to it, if the received ID is an empty string,
// it will return the default defined scheme
func GetSchemeByIDWithDefault(id string) (scheme Scheme, err error) {
	if id == "" {
		id = DefaultSchemeID
	}

	sch, ok := GetSchemeByID(id)
	if !ok {
		return Scheme{}, fmt.Errorf("scheme is not valid")
	}

	return sch, nil
}

// ListSchemes will return a slice of valid scheme ids
func ListSchemes() (schemeIDs []string) {
	for _, t := range schemes {
		schemeIDs = append(schemeIDs, t.ID)
	}

	return schemeIDs
}

// ReadSchemeByEnv allows the user to retrieve the scheme configuration looking by the ID set on an
// environmental variable. It will return a boolean which indicates if the scheme was found or not.
// If the env var is an empty string, it will use the default scheme ID.
func ReadSchemeByEnv() (Scheme, bool) {
	id := os.Getenv("SCHEME_ID")
	if id == "" {
		id = DefaultSchemeID
	}

	return GetSchemeByID(id)
}

// GetSchemeFromEnv allows the user to retrieve the scheme configuration looking by the ID set on an
// environmental variable. If the scheme is not found, function will panic.
func GetSchemeFromEnv() Scheme {
	sch, ok := ReadSchemeByEnv()
	if !ok {
		panic("scheme is not valid")
	}

	return sch
}
