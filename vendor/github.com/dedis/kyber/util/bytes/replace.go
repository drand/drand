package bytes

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
)

// Replacer provides functionalities to replace a file in a safe concurrent way.
// Operations are not carried out until Close is called.
type Replacer struct {
	Name string      // Name of the file being replaced
	Info os.FileInfo // Timestamp of target file before replacement
	File *os.File    // Temporary file used for writing
}

var errRace = errors.New("File was concurrently modified")

// Open a file to replace an existing file as safely as possible,
// attempting to avoid corruption on crash or concurrent writers.
//
// Creates a new temporary file in the same directory as the target,
// but does not actually touch the target until Close() is called.
// The caller can also call Abort() to close and delete the temporary file
// without replacing the original file.
//
// If r.Info is nil, sets it to the status of the target file
// at the time of this Open() call, for later race detection on Close().
// The caller can pre-set r.Info to a different timestamp
// to define the race-detection window explicitly:
// e.g., using an earlier timestamp obtained
// when the original file was first read.
//
// Something like this functionality might be useful to have in os.ioutil.
func (r *Replacer) Open(filename string) error {
	r.Name = filename

	// Save a time-stamp of the file we're replacing
	// so we can detect concurrent modifications.
	if r.Info == nil {
		info, err := os.Stat(filename)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
		}
		r.Info = info
	}

	// Create a temporary file in the same directory for the replacement
	dir := filepath.Dir(filename)
	pfx := filepath.Base(filename)
	file, err := ioutil.TempFile(dir, pfx)
	if err != nil {
		return err
	}
	r.File = file

	return nil
}

// Commit attempts to commit the temporary replacement file to the target.
// On success, Close()s the temporary file and atomically renames it
// to the target filename saved in the Name field.
//
// Returns an error if the target was modified by someone else
// in the time since the temporary file was created.
// In this case, the caller might for example call Abort()
// and create a new Replacer after accounting for the new modifications.
// Alternatively the caller might call ForceCommit() instead after
// warning the user that state might be lost and prompting for confirmation.
func (r *Replacer) Commit() error {

	// Make sure the file hasn't been concurrently modified.
	// Since this check is not atomic with the Rename below,
	// a race can still happen; this is only a heuristic.
	// OS-specific facilities would be needed to make it truly atomic.
	if r.Info != nil {
		info, err := os.Stat(r.Name)
		if err != nil {
			return err
		}
		if info.Name() != r.Info.Name() ||
			info.Size() != r.Info.Size() ||
			info.ModTime() != r.Info.ModTime() {
			return errRace
		}
	}

	// Close the tmpfile
	tmpname := r.File.Name()
	if err := r.File.Close(); err != nil {
		return err
	}

	// Atomically update the target filename
	if err := os.Rename(tmpname, r.Name); err != nil {
		return err
	}

	r.File = nil
	return nil
}

// ForceCommit the temporary replacement file
// without checking for concurrent modifications in the meantime.
func (r *Replacer) ForceCommit() error {
	r.Info = nil
	return r.Commit()
}

// Abort replacement by closing and deleting the temporary file.
// It is harmless to call Abort() after the Replacer
// has already committed or aborted:
// thus, the caller may wish to 'defer r.Abort()' immediately after Open().
func (r *Replacer) Abort() {
	if r.File != nil {
		tmpname := r.File.Name()
		_ = r.File.Close()
		r.File = nil
		_ = os.Remove(tmpname)
	}
}

// IsRace returns true if an error returned by Commit() indicates
// that the commit failed because a concurrent write was detected
// and the force flag was not specified.
func IsRace(err error) bool {
	return err == errRace
}
