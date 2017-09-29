[![Docs](https://img.shields.io/badge/docs-current-brightgreen.svg)](https://godoc.org/gopkg.in/dedis/kyber.v1)
[![Build Status](https://travis-ci.org/dedis/kyber.svg?branch=v1)](https://travis-ci.org/dedis/kyber)

DeDiS Advanced Crypto Library for Go
====================================

This package provides a toolbox of advanced cryptographic primitives for Go,
targeting applications like [Dissent](http://dedis.cs.yale.edu/dissent/)
that need more than straightforward signing and encryption.
Please see the
[GoDoc documentation for this package](http://godoc.org/gopkg.in/dedis/kyber.v1)
for details on the library's purpose and API functionality.

Installing
----------

First make sure you have [Go](https://golang.org)
version 1.3 or newer installed.

The basic crypto library requires only Go and a few
third-party Go-language dependencies that can be installed automatically
as follows:

	go get gopkg.in/dedis/kyber.v1
	cd $GOPATH/src/gopkg.in/dedis/kyber.v1
	go get ./... # install 3rd-party dependencies

You should then be able to test its basic function as follows:

	go test -v

You can recursively test all the packages in the library as follows,
keeping in mind that some sub-packages will only build
if certain dependencies are satisfied as described below:

	go test -v ./...

Constant Time Implementation
----------------------------

By default, this package builds groups that implements constant time arithmetic
operations. The current v1 version only supports the edwards25519 group.  If you
want to have access to variable time arithmetic groups such as P256 or
Curve25519, you need to build the repository with the "vartime" tag:

    go build -tags vartime

And you can test the vartime packages with:

    go test -tags vartime ./...


Migration from v0
-----------------

The v1 is essentially a large clean up of the v0 version, with only a few API
changes, so only minor changes are required.  

+ All references to `abstract.XXX` are now moved up to the top level
  `kyber.XXX`. For example, v1 uses now `kyber.Group` instead of
  `abstract.Group`.
+ `kyber.Suite` do not exist anymore. Now each package should declare its own
  top level package `Suite` interface declaring the functionalities needed by
  the package. One example is the `share/vss` package:
  ```go
      // Suite defines the capabilities required by the vss package.
      type Suite interface {
          kyber.Group
          kyber.CipherFactory
          kyber.HashFactory
      }
  ```
+ `Cipher(key []byte. opts ...interface{}) Cipher` is now `kyber.CipherFactory`.
+ `Hash() hash.Hash` is now `kyber.HashFactory`.
+ The order of arguments for `Point.Mul()` has changed. It now follows the
  mathematical additive notation with the scalar in front:
  `Mul(kyber.Scalar, kyber.Point) kyber.Point`

+ Some packages, structs and methods have been renamed:
    - `ed25519` to `group/edwards25519`
    - `config/KeyPair` to `util/key/Pair`
    - `proof/DLEQProof` -> `proof/dleq/Proof`

+ Many utility functions have been moved to `util/`. For example, the `subtle`
  package in now in `util/subtle/`.

Please, read the CHANGELOG for an exhaustive list of changes.

Issues
------

- Traditionally, ECDH (Elliptic curve Diffie-Hellman) derives the shared secret
from the x point only. In this framework, you can either manually retrieve the
value or use the MarshalBinary method to take the combined (x, y) value as the
shared secret. We recommend the latter process for new softare/protocols using
this framework as it is cleaner and generalizes across different types of
groups (e.g., both integer and elliptic curves), although it will likely be
incompatible with other implementations of ECDH.
http://en.wikipedia.org/wiki/Elliptic_curve_Diffie%E2%80%93Hellman

