// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package edwards25519

import (
	"crypto/sha512"
)

// PrivateKeyToCurve25519 converts an ed25519 private key into a corresponding
// curve25519 private key such that the resulting curve25519 public key will
// equal the result from PublicKeyToCurve25519.
func PrivateKeyToCurve25519(curve25519Private *[32]byte, privateKey *[64]byte) {
	h := sha512.New()
	_, _ = h.Write(privateKey[:32])
	digest := h.Sum(nil)

	digest[0] &= 248
	digest[31] &= 127
	digest[31] |= 64

	copy(curve25519Private[:], digest)
}

func edwardsToMontgomeryX(outX, y *fieldElement) {
	// We only need the x-coordinate of the curve25519 point, which I'll
	// call u. The isomorphism is u=(y+1)/(1-y), since y=Y/Z, this gives
	// u=(Y+Z)/(Z-Y). We know that Z=1, thus u=(Y+1)/(1-Y).
	var oneMinusY fieldElement
	feOne(&oneMinusY)
	feSub(&oneMinusY, &oneMinusY, y)
	feInvert(&oneMinusY, &oneMinusY)

	feOne(outX)
	feAdd(outX, outX, y)

	feMul(outX, outX, &oneMinusY)
}

// PublicKeyToCurve25519 converts an Ed25519 public key into the curve25519
// public key that would be generated from the same private key.
func PublicKeyToCurve25519(curve25519Public *[32]byte, publicKey *[32]byte) bool {
	var A extendedGroupElement
	if !A.FromBytes(publicKey[:]) {
		return false
	}

	// A.Z = 1 as a postcondition of FromBytes.
	var x fieldElement
	edwardsToMontgomeryX(&x, &A.Y)
	feToBytes(curve25519Public, &x)
	return true
}

// sqrtMinusA is sqrt(-486662)
var sqrtMinusA = fieldElement{
	12222970, 8312128, 11511410, -9067497, 15300785, 241793, -25456130, -14121551, 12187136, -3972024,
}

// sqrtMinusHalf is sqrt(-1/2)
var sqrtMinusHalf = fieldElement{
	-17256545, 3971863, 28865457, -1750208, 27359696, -16640980, 12573105, 1002827, -163343, 11073975,
}

// halfQMinus1Bytes is (2^255-20)/2 expressed in little endian form.
var halfQMinus1Bytes = [32]byte{
	0xf6, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x3f,
}

// feBytesLess returns one if a <= b and zero otherwise.
func feBytesLE(a, b *[32]byte) int32 {
	equalSoFar := int32(-1)
	greater := int32(0)

	for i := uint(31); i < 32; i-- {
		x := int32(a[i])
		y := int32(b[i])

		greater = (^equalSoFar & greater) | (equalSoFar & ((x - y) >> 31))
		equalSoFar = equalSoFar & (((x ^ y) - 1) >> 31)
	}

	return int32(^equalSoFar & 1 & greater)
}

// ScalarBaseMult computes a curve25519 public key from a private key and also
// a uniform representative for that public key. Note that this function will
// fail and return false for about half of private keys.
// See http://elligator.cr.yp.to/elligator-20130828.pdf.
func pointToRep(publicKey, representative *[32]byte, A *extendedGroupElement) bool {
	var inv1 fieldElement
	feSub(&inv1, &A.Z, &A.Y)
	feMul(&inv1, &inv1, &A.X)
	feInvert(&inv1, &inv1) // inv1 <- 1/X(Z-Y) = 1/x(1-y)Z^2

	var t0, u fieldElement
	feMul(&u, &inv1, &A.X) // u <- X/X(Z-Y) = 1/(1-y)Z
	feAdd(&t0, &A.Y, &A.Z) // t0 <- Y+Z = (1+y)Z
	feMul(&u, &u, &t0)     // u <- (1+y)/(1-y)

	var v fieldElement
	feMul(&v, &t0, &inv1)
	feMul(&v, &v, &A.Z)
	feMul(&v, &v, &sqrtMinusA) // v <- sqrt(-A)Z(Z+Y)/X(Z-Y)
	// (x-coord on Montgomery curve)

	var b fieldElement
	feAdd(&b, &u, &paramA) // b <- u+A

	var c, b3, b8 fieldElement
	feSquare(&b3, &b)   // 2
	feMul(&b3, &b3, &b) // 3
	feSquare(&c, &b3)   // 6
	feMul(&c, &c, &b)   // 7
	feMul(&b8, &c, &b)  // 8

	feMul(&c, &c, &u) // c <- b^7u = u(u+A)^7
	q58(&c, &c)       // c <- (u(u+A)^7)^((p-5)/8)
	//					= (u^((p-5)/8))((u+A)^(7(p-5)/8))
	//					= (u^((p-5)/8))((u+A)^(7(p-5)/8))
	//					= sqrt(b^7u)-1

	//	a^((p+3)/8) = sqrt(a)
	//	a^((p-5)/8) = a^((p+3-8)/8) = a^((p+3)/8-1) = sqrt(a)-1

	var chi fieldElement
	feSquare(&chi, &c)
	feSquare(&chi, &chi)

	feSquare(&t0, &u)
	feMul(&chi, &chi, &t0)

	feSquare(&t0, &b)   // 2
	feMul(&t0, &t0, &b) // 3
	feSquare(&t0, &t0)  // 6
	feMul(&t0, &t0, &b) // 7
	feSquare(&t0, &t0)  // 14
	feMul(&chi, &chi, &t0)
	feNeg(&chi, &chi)

	var chiBytes [32]byte
	feToBytes(&chiBytes, &chi)
	// chi[1] is either 0 or 0xff
	if chiBytes[1] == 0xff {
		return false
	}

	// Calculate r1 = sqrt(-u/(2*(u+A)))
	var r1 fieldElement
	feMul(&r1, &c, &u)
	feMul(&r1, &r1, &b3)
	feMul(&r1, &r1, &sqrtMinusHalf)

	var maybeSqrtM1 fieldElement
	feSquare(&t0, &r1)
	feMul(&t0, &t0, &b)
	feAdd(&t0, &t0, &t0)
	feAdd(&t0, &t0, &u)

	feOne(&maybeSqrtM1)
	feCMove(&maybeSqrtM1, &sqrtM1, feIsNonZero(&t0))
	feMul(&r1, &r1, &maybeSqrtM1)

	// Calculate r = sqrt(-(u+A)/(2u))
	var r fieldElement
	feSquare(&t0, &c)   // 2
	feMul(&t0, &t0, &c) // 3
	feSquare(&t0, &t0)  // 6
	feMul(&r, &t0, &c)  // 7

	feSquare(&t0, &u)   // 2
	feMul(&t0, &t0, &u) // 3
	feMul(&r, &r, &t0)

	feSquare(&t0, &b8)   // 16
	feMul(&t0, &t0, &b8) // 24
	feMul(&t0, &t0, &b)  // 25
	feMul(&r, &r, &t0)
	feMul(&r, &r, &sqrtMinusHalf)

	feSquare(&t0, &r)
	feMul(&t0, &t0, &u)
	feAdd(&t0, &t0, &t0)
	feAdd(&t0, &t0, &b)
	feOne(&maybeSqrtM1)
	feCMove(&maybeSqrtM1, &sqrtM1, feIsNonZero(&t0))
	feMul(&r, &r, &maybeSqrtM1)

	var vBytes [32]byte
	feToBytes(&vBytes, &v)
	vInSquareRootImage := feBytesLE(&vBytes, &halfQMinus1Bytes)
	feCMove(&r, &r1, vInSquareRootImage)

	feToBytes(publicKey, &u)
	feToBytes(representative, &r)
	return true
}

// q58 calculates out = z^((p-5)/8).
func q58(out, z *fieldElement) {
	var t1, t2, t3 fieldElement
	var i int

	feSquare(&t1, z)        // 2^1
	feMul(&t1, &t1, z)      // 2^1 + 2^0
	feSquare(&t1, &t1)      // 2^2 + 2^1
	feSquare(&t2, &t1)      // 2^3 + 2^2
	feSquare(&t2, &t2)      // 2^4 + 2^3
	feMul(&t2, &t2, &t1)    // 4,3,2,1
	feMul(&t1, &t2, z)      // 4..0
	feSquare(&t2, &t1)      // 5..1
	for i = 1; i < 5; i++ { // 9,8,7,6,5
		feSquare(&t2, &t2)
	}
	feMul(&t1, &t2, &t1)     // 9,8,7,6,5,4,3,2,1,0
	feSquare(&t2, &t1)       // 10..1
	for i = 1; i < 10; i++ { // 19..10
		feSquare(&t2, &t2)
	}
	feMul(&t2, &t2, &t1)     // 19..0
	feSquare(&t3, &t2)       // 20..1
	for i = 1; i < 20; i++ { // 39..20
		feSquare(&t3, &t3)
	}
	feMul(&t2, &t3, &t2)     // 39..0
	feSquare(&t2, &t2)       // 40..1
	for i = 1; i < 10; i++ { // 49..10
		feSquare(&t2, &t2)
	}
	feMul(&t1, &t2, &t1)     // 49..0
	feSquare(&t2, &t1)       // 50..1
	for i = 1; i < 50; i++ { // 99..50
		feSquare(&t2, &t2)
	}
	feMul(&t2, &t2, &t1)      // 99..0
	feSquare(&t3, &t2)        // 100..1
	for i = 1; i < 100; i++ { // 199..100
		feSquare(&t3, &t3)
	}
	feMul(&t2, &t3, &t2)     // 199..0
	feSquare(&t2, &t2)       // 200..1
	for i = 1; i < 50; i++ { // 249..50
		feSquare(&t2, &t2)
	}
	feMul(&t1, &t2, &t1) // 249..0
	feSquare(&t1, &t1)   // 250..1
	feSquare(&t1, &t1)   // 251..2
	feMul(out, &t1, z)   // 251..2,0
}

// chi calculates out = z^((p-1)/2). The result is either 1, 0, or -1 depending
// on whether z is a non-zero square, zero, or a non-square.
// This could probably be computed more quickly with Euclid's algorithm,
// blinded for constant-time.
func chi(out, z *fieldElement) {
	var t0, t1, t2, t3 fieldElement
	var i int

	feSquare(&t0, z)        // 2^1
	feMul(&t1, &t0, z)      // 2^1 + 2^0
	feSquare(&t0, &t1)      // 2^2 + 2^1
	feSquare(&t2, &t0)      // 2^3 + 2^2
	feSquare(&t2, &t2)      // 4,3
	feMul(&t2, &t2, &t0)    // 4,3,2,1
	feMul(&t1, &t2, z)      // 4..0
	feSquare(&t2, &t1)      // 5..1
	for i = 1; i < 5; i++ { // 9,8,7,6,5
		feSquare(&t2, &t2)
	}
	feMul(&t1, &t2, &t1)     // 9,8,7,6,5,4,3,2,1,0
	feSquare(&t2, &t1)       // 10..1
	for i = 1; i < 10; i++ { // 19..10
		feSquare(&t2, &t2)
	}
	feMul(&t2, &t2, &t1)     // 19..0
	feSquare(&t3, &t2)       // 20..1
	for i = 1; i < 20; i++ { // 39..20
		feSquare(&t3, &t3)
	}
	feMul(&t2, &t3, &t2)     // 39..0
	feSquare(&t2, &t2)       // 40..1
	for i = 1; i < 10; i++ { // 49..10
		feSquare(&t2, &t2)
	}
	feMul(&t1, &t2, &t1)     // 49..0
	feSquare(&t2, &t1)       // 50..1
	for i = 1; i < 50; i++ { // 99..50
		feSquare(&t2, &t2)
	}
	feMul(&t2, &t2, &t1)      // 99..0
	feSquare(&t3, &t2)        // 100..1
	for i = 1; i < 100; i++ { // 199..100
		feSquare(&t3, &t3)
	}
	feMul(&t2, &t3, &t2)     // 199..0
	feSquare(&t2, &t2)       // 200..1
	for i = 1; i < 50; i++ { // 249..50
		feSquare(&t2, &t2)
	}
	feMul(&t1, &t2, &t1)    // 249..0
	feSquare(&t1, &t1)      // 250..1
	for i = 1; i < 4; i++ { // 253..4
		feSquare(&t1, &t1)
	}
	feMul(out, &t1, &t0) // 253..4,2,1
}

// RepresentativeToPublicKey converts a uniform representative value for a
// curve25519 public key, as produced by ScalarBaseMult, to a curve25519 public
// key.
// See Elligator paper section 5.2.
func repToCurve25519(publicKey, representative *[32]byte) {
	var rr2, v, e fieldElement
	feFromBytes(&rr2, representative[:])

	// v = -A/(1+ur^2), where u = 2 (any non-square field element)
	feSquare2(&rr2, &rr2)
	rr2[0]++
	feInvert(&rr2, &rr2)
	feMul(&v, &paramA, &rr2)
	feNeg(&v, &v)

	// e = Chi(v^3 + Av^2 + Bv), where B = 1
	var v2, v3 fieldElement
	feSquare(&v2, &v)
	feMul(&v3, &v, &v2)
	feAdd(&e, &v3, &v)
	feMul(&v2, &v2, &paramA)
	feAdd(&e, &v2, &e)
	chi(&e, &e)
	var eBytes [32]byte
	feToBytes(&eBytes, &e)
	// eBytes[1] is either 0 (for e = 1) or 0xff (for e = -1)
	eIsMinus1 := int32(eBytes[1]) & 1

	// x = ev - (1-e)A/2
	var negV fieldElement
	feNeg(&negV, &v)
	feCMove(&v, &negV, eIsMinus1) // v <- ev
	feZero(&v2)
	feCMove(&v2, &paramA, eIsMinus1) // v2 <- (1-e)A/2 (= 0 or A)
	feSub(&v, &v, &v2)

	feToBytes(publicKey, &v) // Curve25519 pubkey
}
