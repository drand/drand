package util

import "errors"

// Concat combines multiple arrays of the same type into a single array
func Concat[T any](arrs ...[]T) []T {
	var out []T
	for _, arr := range arrs {
		out = append(out, arr...)
	}
	return out
}

// Cont is contains, but the util package already contains another func just for `Participant`
func Cont[T comparable](haystack []T, needle T) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

func First[T any](haystack []T, predicate func(T) bool) (*T, error) {
	for _, value := range haystack {
		if predicate(value) {
			return &value, nil
		}
	}
	return nil, errors.New("item not found in array")
}

func Filter[T any](arr []T, predicate func(T) bool) []T {
	var out []T
	for _, v := range arr {
		if predicate(v) {
			out = append(out, v)
		}
	}
	return out
}