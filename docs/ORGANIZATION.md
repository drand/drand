# Drand Code Organization

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Top level packages

* `chain` - code for generating the sequence of beacons (implementation of which is in `chain/beacon`) after setup.
* `client` - the drand client library - composition utilities for fail-over and reliable abstraction
  * `client/grpc` - the concrete gRPC client implementation
  * `client/http` - the concrete HTTP client implementation
  * `client/test` - mock client implementations for testing
* `cmd` - the binary entry points
  * `cmd/client` - a client for fetching randomness
  * `cmd/client/lib` - a common library for creating a client shared by `cmd/client` and `cmd/relay`
  * `cmd/drand-cli` - the main drand binary
  * `cmd/relay` - a relay that pulls randomness from a drand group member and exposes an HTTP server interface
  * `cmd/relay-gossip` - a relay that pulls randomness from a group member and publishes it over a gossipsub topic
* `core` - The primary Service interface of drand comamands
* `demo` - A framework for integration testing
* `deploy` - Records of previous drand deployments
* `docker` - Helpers for docker image packaging
* `docs` - Here
* `entropy` - A common abstraction for ingesting randomness
* `fs` - Utilities for durable state storage
* `hooks` - Docker helper entrypoint
* `http` - The publicly exposed HTTP server for exposing randomness
* `key` - Validation of signatures
* `log` - Common logging library
* `lp2p` - utilities for constructing a libp2p host
  * `lp2p/client` - the concrete gossip client implementation
* `metrics` - The prometheus metrics server
* `net` - gRPC service handlers for inter-node communication
* `protobuf/drand` - definitions for the wire format interface of inter-node communication
* `test` - testing helper utilities
