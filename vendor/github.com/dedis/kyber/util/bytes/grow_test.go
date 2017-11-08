package bytes

import (
	"bytes"
	"testing"
)

var growTests = []struct {
	s    []byte
	n    int
	want []byte
}{
	{[]byte{}, 0, []byte{}},
	{[]byte{}, 1, []byte{0}},
	{[]byte("abcdef"), 3, []byte{0, 0, 0}},
	{[]byte("abcdef")[:3], 3, []byte("def")},
	{[]byte("abcdef")[:3], 6, []byte{0, 0, 0, 0, 0, 0}},
}

func TestGrow(t *testing.T) {
	for _, tt := range growTests {
		ns, ext := Grow(tt.s, tt.n)
		if len(ns) != len(tt.s)+tt.n {
			t.Errorf("Grow(%q, %q): len(ns) = %v, want %v", tt.s, tt.n, len(ns), len(tt.s)+tt.n)
		}
		if !bytes.Equal(ns[:len(tt.s)], tt.s) {
			t.Errorf("Grow(%q, %q): ns = %v, want %v", tt.s, tt.n, ns[:len(tt.s)], tt.s)
		}
		if !bytes.Equal(ext, tt.want) {
			t.Errorf("Grow(%q, %q): ext = %v, want %v", tt.s, tt.n, ext, tt.want)
		}
	}
}
