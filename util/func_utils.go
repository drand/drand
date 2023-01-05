package util

// Concat combines multiple arrays of the same type into a single array
func Concat[T any](arrs ...[]T) []T {
	var out []T
	for _, arr := range arrs {
		out = append(out, arr...)
	}
	return out
}
