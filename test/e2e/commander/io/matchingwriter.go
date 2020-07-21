package io

import "strings"

// MatchingWriter is a io.Writer that matches on a substr
type MatchingWriter struct {
	substr string
	C      chan string
}

// NewMatchingWriter creates a writer and writes the matched data to it's
// channel when data is written that contains the passed substr.
func NewMatchingWriter(substr string) *MatchingWriter {
	matches := make(chan string)
	return &MatchingWriter{substr, matches}
}

// Write checks to see if the written bytes contain the substr and sends the
// bytes to the channel if they do.
func (mw *MatchingWriter) Write(p []byte) (n int, err error) {
	data := string(p)
	if strings.Contains(data, mw.substr) {
		select {
		case mw.C <- data:
		default:
		}
	}
	return len(p), nil
}
