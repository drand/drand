all: test

# gopkg fits all v1.1, v1.2, ... in v1
PKG_STABLE = gopkg.in/dedis/kyber.v1
include $(GOPATH)/src/github.com/dedis/Coding/bin/Makefile.base

# You can use `test_playground` to run any test or part of cothority
# for more than once in Travis. Change `make test` in .travis.yml
# to `make test_playground`.
test_playground:
	cd skipchain; \
	for a in $$( seq 100 ); do \
	  go test -v -race -short || exit 1 ; \
	done;

# Other targets are:
# make create_stable
