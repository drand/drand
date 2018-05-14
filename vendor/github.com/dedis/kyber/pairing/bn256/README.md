bn256
-----

Package bn256 implements the Optimal Ate pairing over a 256-bit Barreto-Naehrig
curve targeting a 128-bit security level as described in the paper 
[New Software Speed Records for Cryptocraphic Pairings](http://cryptojedi.org/papers/dclxvi-20100714.pdf). 
Its output is compatible with the implementation described in that paper.

The basis for this package is [Cloudflare's bn256 implementation](https://github.com/cloudflare/bn256)
which itself is an improved version of the [official bn256 package](https://golang.org/x/crypto/bn256).
The package at hand maintains compatibility to Cloudflare's library. The biggest difference is the replacement of their
[public API](https://github.com/cloudflare/bn256/blob/master/bn256.go) by a new
one that is compatible to Kyber's scalar, point, group, and suite interfaces.

[Bilinear groups](https://en.wikipedia.org/wiki/Pairing-based_cryptography) are
the basis for many new cryptographic protocols that have been proposed over the
past decade. They consist of a triplet of groups (G₁, G₂ and GT) such that there
exists a function e(g₁ˣ,g₂ʸ)=gTˣʸ (where gₓ is a generator of the respective
group) which is called a pairing.


