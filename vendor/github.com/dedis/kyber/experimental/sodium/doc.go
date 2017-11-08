// +build experimental

// +build sodium

/*
Package sodium contains functionality ported from the Sodium crypto library.
See http://doc.libsodium.org for background and details on Sodium.

Currently all of the code in these Sodium-derived sub-packages
are extremely experimental and may (or may not) go away in the future.
These consist of alternative implementations of functionality
that is also implemented in other ways in other parts of the library;
whether the Sodium-based implementations should be retained
depends in part on whether they end up offering significant added value,
such as (in particular) better performance than pure Go-based implementations.

The rest of the crypto library currently does not depend on these packages,
and should not until a definite decision is made.
*/
package sodium
