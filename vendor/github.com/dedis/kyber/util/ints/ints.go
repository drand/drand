// Package ints provide some utility functions to deal with integers.
//
package ints

// Max returns the maximum of its arguments.
func Max(x int, y ...int) int {
	for _, z := range y {
		if z > x {
			x = z
		}
	}
	return x
}

// Min returns the minimum of its arguments.
func Min(x int, y ...int) int {
	for _, z := range y {
		if z < x {
			x = z
		}
	}
	return x
}

// Abs returns |x|, the absolute value of x.
func Abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Sign returns:
//
//	-1 if x <  0
//	 0 if x == 0
//	+1 if x >  0
//
func Sign(x int) int {
	if x < 0 {
		return -1
	} else if x > 0 {
		return 1
	}
	return 0
}
