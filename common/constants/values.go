package constants

import "os"

// DefaultBeaconID is the value used when beacon id has an empty value. This
// value should not be changed for backward-compatibility reasons
const DefaultBeaconID = "default"

// GetBeaconIDFromEnv read beacon id from an environmental variable.
// It is used for testing purpose.
func GetBeaconIDFromEnv() string {
	return os.Getenv("BEACON_ID")
}
