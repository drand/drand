package io

import (
	"fmt"
	"io"
	"os"
)

type prefixedWriter struct {
	prefix string
	writer io.Writer
}

// PrefixedWriter creates a new io.Writer that writes to the passed writer after
// appending the passed prefix string.
func PrefixedWriter(pfx string, w io.Writer) io.Writer {
	if w == nil {
		w = os.Stdout
	}
	return &prefixedWriter{prefix: pfx, writer: w}
}

func (pw *prefixedWriter) Write(p []byte) (n int, err error) {
	str := fmt.Sprintf("[%s] %s", pw.prefix, p)
	return pw.writer.Write([]byte(str))
}
