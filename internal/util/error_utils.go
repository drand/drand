package util

import "strings"

func ErrorContains(err error, str string) bool {
	return strings.Contains(err.Error(), str)
}
