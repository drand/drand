# Warning

**The OpenSSL package is flagged experimental. Only use it if absolutely necessary. We recommend to use the NIST package instead which offers the same functionality and more importantly is stable.**

Problems with the OpenSSL package include:
- No thread safety.
- Some computations produce different results than the NIST package (using the same underlying crypto primitives).
- The default OpenSSL library shipped with OSX El Capitan does not work with this package. Install and link the latest version e.g. through
```
brew install openssl && brew unlink openssl && brew link openssl --overwrite --force
```

and then change in `dedis/kyber/openssl/aes.go` the macro

```
// #cgo CFLAGS: -Wno-deprecated
```

to

```
// #cgo CFLAGS: -Wno-deprecated -I/usr/local/include
```

