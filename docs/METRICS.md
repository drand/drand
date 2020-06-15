<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**

- [Drand Metrics](#drand-metrics)
  - [Local Metrics](#local-metrics)
  - [Shared Group Metrics](#shared-group-metrics)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# Drand Metrics

Drand uses [prometheus](https://prometheus.io/) instrumentation for helping
operators monitor and understand the runtime behavior of system.

## Local Metrics

The local drand node exposes metrics on an HTTP server listening as specified
by the `--metrics` comamnd line flag. You can view the reported metrics
in a browser at `http://localhost:<metrics port>/metrics`. This page includes

- The default Golang process metrics collected by prometheus
- Statistics on the drand beacon and group behavior
- Statistics on the HTTP public listener request load if enabled.

## Shared Group Metrics

In addition to the metrics collected within the local node, the drand
GRPC group protocol supports re-export and sharing of group metrics
between group members. When a local metrics port is specified,
metrics shared by other group members can be accessed at
`http://localhost:<metrics port>/peer/<peer address>/metrics`.
This will only inlclude the drand beacon statistics shared by the
remote peer, and does not include the process or internal health of
the node. It is meant to allow better visibility when debugging
network issues, and helping operators understand where problems
originate.
