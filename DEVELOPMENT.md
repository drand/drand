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

### Development environment

Certain features in Drand depend on external services.
These features are the support for running with PostgreSQL as a database backend, and observability features such as metrics, and tracing.

To keep your environment clean from any external tools required to interact with such features, you can use the
`docker-compose.yaml` file under `devenv/docker-compose.yaml`.

#### Using the devenv tools

To launch the tools, run
```shell
cd devenv
docker compose up -d
```

If you wish to stop the stack, run:
```shell
docker compose down
```

To cleanup and remove all data, run:
```shell
cd devenv
docker compose down --volumes --remove-orphans
```

#### PostgreSQL backend

To use the database instance provided with the devenv, use `127.0.0.1:5432` as the destination for PostgreSQL.

For more details, see the [testing section below](#testing-with-postgresql-as-database-backend).

#### Observability features

Drand can produce traces compatible with OpenTelemetry specification. To turn on this feature, set the `DRAND_TRACES`
environment varible to the desired destination, e.g.
```shell
export DRAND_TRACES=127.0.0.1:4317
export DRAND_TRACES_PROBABILITY=1 # This will sample all traces to the destination server
```

After that, in the same terminal, use any of the drand features, such as `make test-unit-memdb`, to start producing traces.

To explore the trace details, launch a new browser tab/window at the [Grafana instance](http://127.0.0.1:3000/explore?orgId=1),
which will allow you to explore in detail the inner workings of Drand.

For more details on how to use Grafana, you can [read the manual here](https://grafana.com/docs/grafana/v9.4/explore/trace-integration/).

### Development flow

After editing all files required by your change:

- Run all tests using `make test`. This will run both unit and integration tests.
- Check your code coverage using `make coverage`.
- Check that all code still compiles using `make build_all`.
- Test operations still work using the `test/local.sh` script. You can terminate it using the CTRL+C/SIGINT and will clean all spawned processes.

You can also run regression testing, see [the section below](#regression-testing).

### Testing with Docker Compose

To test changes using Docker Compose, navigate to [Docker Readme](internal/test/docker/README.md) and follow the steps described there.

#### Testing with in-memory storage as database backend

Drand supports running with an in-memory storage provider for storing beacons.

To check your code against it, run `make test-unit-memdb`.

You can also run the `make demo-memdb` command to launch the scripted demo using
in-memory storage as a backend.

#### Testing with PostgreSQL as database backend

Drand supports PostgreSQL as a database backend for storing beacons.

To check your code against it, run `make test-unit-postgres`.

You can also run the `make demo-postgres` command to launch the scripted demo using
PostgreSQL as a backend.

If you want to run an isolated version of Postgres, you can use the `devenv/docker-compose.yaml` file
from the root of this repository to do so.

To start the database, use:
```shell
cd devenv/
docker compose up -d
```

To stop the database, use:
```shell
docker compose down
```

If you wish to remove the database volume too, use this command instead to stop:
```shell
docker compose down --volumes --remove-orphans
```

## Regression testing

To make sure new changes can integrate without issues with the existing deployments,
you can run regression testing.

To do so, run the following commands:
```shell
git checkout master
go build -o drand-existing
git checkout <your-branch>
go build -o drand-candidate
go run ./demo/regression -release ./drand-existing -candidate ./drand-candidate
```

If you want to test your code against the PostgreSQL backend, replace the
`go run` command above with:

```shell
go run ./demo/regression -db=postgres -release ./drand-existing -candidate ./drand-candidate
```
