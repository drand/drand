package rand

import (
    "golang.org/x/crypto/sha3"
    "hash"
)

// Hash helpers

// HashBytes -- the hash of the concatenation of multiple byte arrays
// This calculates the SHA3-256 value
// Access the digest for example with h := d.Sum(nil)
func HashBytes(b ...[]byte) hash.Hash {
    d := sha3.New256()
    for _, bi := range b {
        d.Write(bi)
    }
    return d
}

