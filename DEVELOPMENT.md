# Quick Developer Guide

If you are reading this, it means you are about to work on the codebase.

## Table of Contents
- [Getting started](#getting-started)
  - [Installing dependencies](#installing-dependencies) 
  - [Development flow](#development-flow)
- [Open issues](#open-issues)

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
- Test operations still work using ....
  - Note: Canceling the `test/local.sh` does not correctly shutdown the instances at the moment.

## Open issues

This is a list of a few known issues with the current codebase:

- Tests should run with `-count 1` to avoid caching of results. It will result in longer running tests but remove any potentially hidden bugs which are intermittent.
- `makefile` is inconsistently running tests
- There are numerous mismatch comments with their functions' signatures
- Store interface definition https://github.com/drand/drand/blob/883cabcec77adf1cad7e2a1afb3d8f5cba7567d8/chain/boltdb/store.go#L155 assumes implementation https://github.com/drand/drand/blob/883cabcec77adf1cad7e2a1afb3d8f5cba7567d8/core/drand_beacon_control.go#L299 rather than the other way around
- False sharing/data locking protection issues, e.g.
```go
    type BeaconHandler struct {
        // NOTE: should only be accessed via getChainInfo
        chainInfo   *chain.Info
        // ...
    }

    // ...

    bh.chainInfoLk.RLock()
    if bh.chainInfo != nil {
        info := bh.chainInfo
        bh.chainInfoLk.RUnlock()
        return info, nil
    }
    bh.chainInfoLk.RUnlock()
```
- Some linting errors:
```
cmd/client/main.go:163:45: G114: Use of net/http serve function that has no support for setting timeouts (gosec)
                        log.DefaultLogger().Fatalw("", "client", http.ListenAndServe(address, nil))
                                                                 ^
cmd/relay/main.go:152:9: G114: Use of net/http serve function that has no support for setting timeouts (gosec)
        return http.Serve(listener, handler.GetHTTPHandler())
               ^
log/log.go:16:13: the interface has more than 10 methods: 14 (interfacebloat)
type Logger interface {
            ^
cmd/drand-cli/cli_test.go:8:2: SA1019: "io/ioutil" has been deprecated since Go 1.16: As of Go 1.16, the same functionality is now provided by package io or package os, and those implementations should be preferred in new code. See the specific function documentation for details. (staticcheck)
        "io/ioutil"
        ^
```

- GoLand reports a few issues that might cause problems, especially around the nilness analyzer.
- Some tests produce errors:

TestLocalOrchestration
```
=== RUN   TestLocalOrchestration

2022-10-03T22:49:32.804+0300    ERROR    127.0.0.1:44703.default.0    beacon/node.go:410        {"beacon_round": 6, "err_request": "rpc error: code = Unavailable desc = connection error: desc = \"transport: Error while dialing dial tcp 127.0.0.1:41087: connect: connection refused\"", "from": "127.0.0.1:41087"}
2022-10-03T22:49:32.804+0300    ERROR    127.0.0.1:44703.default.0.SyncManager.tryNode    beacon/sync_manager.go:358    unable_to_sync    {"with_peer": "127.0.0.1:37611", "err": "rpc error: code = Unavailable desc = connection error: desc = \"transport: Error while dialing dial tcp 127.0.0.1:37611: connect: connection refused\""}
```

TestHTTPWaiting
```
=== RUN   TestHTTPWaiting
2022-10-04T09:42:09.384+0300    INFO    http/server.go:127    New beacon handler registered    {"chainHash": "c5f1466e51ac94baa8a04fc9d5de8cd8f5e0cbb4985b70314e125d41f783b45c"}
MOCK SERVER: emit round done
2022-10-04T09:42:09.390+0300    WARN    http/server.go:390        {"http_server": "request in the future", "client": "[::1]:59544", "req": "%2Fc5f1466e51ac94baa8a04fc9d5de8cd8f5e0cbb4985b70314e125d41f783b45c%2Fpublic%2F1971"}
    server_test.go:202: shouldn't be done. unexpected status: 404
--- FAIL: TestHTTPWaiting (0.07s)
```

Generally running tests with `go test ./...`

```
2022-10-03T22:49:45.024+0300    DEBUG    127.0.0.1:37611.default.1.SyncManager.tryNode    beacon/sync_manager.go:434    sync canceled    {"source": "global", "err?": "context canceled"}
2022-10-03T22:49:45.024+0300    DEBUG    127.0.0.1:37611.default.1.SyncManager    beacon/sync_manager.go:308    sync canceled early    {"source": "ctx", "err?": "context canceled"}
2022-10-03T22:49:45.024+0300    INFO    127.0.0.1:41087.default.2    beacon/node.go:435    beacon handler stopped    {"time": "2022-10-03T22:49:45.024+0300"}
2022-10-03T22:49:45.024+0300    INFO    net/client_grpc.go:245        {"grpc client": "chain sync", "error": "rpc error: code = Canceled desc = context canceled", "to": "127.0.0.1:41087"}
2022-10-03T22:49:45.024+0300    INFO    127.0.0.1:44703.default.0    beacon/node.go:435    beacon handler stopped    {"time": "2022-10-03T22:49:45.024+0300"}
2022-10-03T22:49:45.026+0300    ERROR    node/node_inprocess.go:312        {"drand": "failed to shutdown", "err": "rpc error: code = Unavailable desc = error reading from server: EOF"}
    - Successfully stopped Node 3 ( 127.0.0.1:44703 )
2022-10-03T22:49:45.026+0300    ERROR    node/node_inprocess.go:312        {"drand": "failed to shutdown", "err": "rpc error: code = Unavailable desc = error reading from server: EOF"}
    - Successfully stopped Node 2 ( 127.0.0.1:41087 )
2022-10-03T22:49:45.026+0300    ERROR    node/node_inprocess.go:312        {"drand": "failed to shutdown", "err": "rpc error: code = Unavailable desc = error reading from server: EOF"}
    - Successfully stopped Node 1 ( 127.0.0.1:37611 )
```
