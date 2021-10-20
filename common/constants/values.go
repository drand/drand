package constants

import "os"

const DefaultBeaconID = "default"

func GetBeaconIDFromEnv() string {
	return os.Getenv("BEACON_ID")
}
