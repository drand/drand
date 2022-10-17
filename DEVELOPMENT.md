# Quick Developer Guide

If you are reading this, it means you are about to work on the codebase.

## Table of Contents
- [Getting started](#getting-started)
  - [Installing dependencies](#installing-dependencies) 
  - [Development flow](#development-flow)

## Getting started

### Installing dependencies

To start, you'll need to run a few commands to make sure you have the
correct development environment tools installed:
 
- `make install_deps_<os>` where `<os>` can be `linux` or `macos`. This will install the proto compiler under `/usr/local/bin/protoc`.
- `make install_lint`. This will install `golangci-lint` at the version used during Drand's development.
- `make build_proto`. This will compile the project's proto files.

Finally, if you ran all the above commands and `git status` does not report any changes to the repository,
then you are ready to start.

### Development flow

After editing all files required by your change:

- Run all tests using `make test`. This will run both unit and integration tests.
- Check your code coverage using `make coverage`.
- Check that all code still compiles using `make build_all`.
- Test operations still work using the `test/local.sh` script. You can terminate it using the CTRL+C/SIGINT and will clean all spawned processes.

### Testing with Docker Compose

To test changes using Docker Compose, navigate to [Docker Readme](test/docker/README.md) and follow the steps described there.
