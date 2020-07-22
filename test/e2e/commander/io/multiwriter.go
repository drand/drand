package io

import (
	"io"
	"sync"
)

// MultiWriter is a writer that writes to multiple other writers.
type MultiWriter struct {
	sync.RWMutex
	writers []io.Writer
}

// NewMultiWriter creates a writer that duplicates its writes to all the
// provided writers, similar to the Unix tee(1) command. Writers can be added
// and removed dynamically after creation.
func NewMultiWriter(writers ...io.Writer) *MultiWriter {
	mw := &MultiWriter{writers: writers}
	return mw
}

// Write writes some bytes to all the writers.
func (mw *MultiWriter) Write(p []byte) (n int, err error) {
	mw.RLock()
	defer mw.RUnlock()

	for _, w := range mw.writers {
		n, err = w.Write(p)
		if err != nil {
			return
		}

		if n < len(p) {
			err = io.ErrShortWrite
			return
		}
	}

	return len(p), nil
}

// Add appends a writer to the list of writers this multiwriter writes to.
func (mw *MultiWriter) Add(w io.Writer) {
	mw.Lock()
	mw.writers = append(mw.writers, w)
	mw.Unlock()
}

// Remove will remove a previously added writer from the list of writers.
func (mw *MultiWriter) Remove(w io.Writer) {
	mw.Lock()
	var writers []io.Writer
	for _, ew := range mw.writers {
		if ew != w {
			writers = append(writers, ew)
		}
	}
	mw.writers = writers
	mw.Unlock()
}
