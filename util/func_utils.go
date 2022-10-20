package util

import (
	"time"

	"github.com/drand/drand/log"
)

// RetryOnError calls a function `times` number of times with a second between each or until it returns a nil error
// it returns the last error
func RetryOnError[T any](times int, fn func() (*T, error)) (*T, error) {
	var lastError error
	for i := 0; i < times; i++ {
		res, err := fn()
		if err == nil {
			return res, nil
		}
		log.DefaultLogger().Debugw("retrying...")
		time.Sleep(1 * time.Second)
		lastError = err
	}

	return nil, lastError
}

// Concat combines two arrays of the same type into a single array
func Concat[T any](a, b []T) []T {
	var out []T
	out = append(out, a...)
	out = append(out, b...)
	return out
}
