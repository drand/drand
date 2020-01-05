[![CircleCI](https://circleci.com/gh/drand/bls12-381.svg?style=svg)](https://circleci.com/gh/drand/bls12-381)

**This is a fork implementing a kyber/dedis wrapper for this library (among
other things)**

High speed bls12-381 implementation in go.

_wip_. _Do not use in production_.

- [x] x86 field operations
- [x] extention towers
- [x] group operations
- [x] serialization
- [x] pairing
- [ ] hash to g1 & g2 (pending for [standart](https://github.com/cfrg/draft-irtf-cfrg-bls-signature))
- [x] bls signature scheme
- [ ] arm arch field operations
- [ ] fallback field operations

#### Benchmarks

on _2.7 GHz i5_

```
BenchmarkPairing  1145435 ns/op
```

#### About

This library is ETH2.0 compatible and supported by [Prysmatic Labs](https://prysmaticlabs.com)

##### Authors

Sait İmamoğlu, Onur Kılıç
