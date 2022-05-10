package common

// DefaultBeaconID is the value used when beacon id has an empty value. This
// value should not be changed for backward-compatibility reasons
const DefaultBeaconID = "default"

// DefaultChainHash is the value used when chain hash has an empty value on requests
// from clients. This value should not be changed for
// backward-compatibility reasons
const DefaultChainHash = "default"

// MultiBeaconFolder is the name of the folder where the multi-beacon data is stored
const MultiBeaconFolder = "multibeacon"

// IsDefaultBeaconID indicates if the beacon id received is the default one or not.
// There is a direct relationship between an empty string and the reserved id "default".
// Internally, empty string is translated to "default" so we can create the beacon folder
// with a valid name.
func IsDefaultBeaconID(beaconID string) bool {
	return beaconID == DefaultBeaconID || beaconID == ""
}

// CompareBeaconIDs indicates if two different beacon ids are equivalent or not.
// It handles default values too.
func CompareBeaconIDs(id1, id2 string) bool {
	if IsDefaultBeaconID(id1) && IsDefaultBeaconID(id2) {
		return true
	}

	if id1 != id2 {
		return false
	}

	return true
}

// GetCanonicalBeaconID returns the correct beacon id.
func GetCanonicalBeaconID(id string) string {
	if IsDefaultBeaconID(id) {
		return DefaultBeaconID
	}
	return id
}
