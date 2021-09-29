package utils

import (
	"os"
	"strconv"
)

func PrevSigDecoupling() bool {
	flag, err := strconv.ParseBool(os.Getenv("DECOUPLE_PREV_SIG"))
	if err != nil {
		return false
	}

	return flag
}
