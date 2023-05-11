# Instructions

## Overview

---
The `docker-compose.yml` file will start 5 copies of the drand daemon. They will
listen on the following ports (where `x` is 0-4 depending on the daemon number):

| Port name      | Internal port | External port |
|----------------|---------------|---------------|
| Private listen | 8x80          | 5x30          |
| Public listen  | 8x81          | 5x31          |
| Control        | 8888          | 5x32          |
| Metrics        | 8x83          | 5x33          |

There are scripts in the `utils` directory for the following:

* `startBeacon.sh` - Runs the initial DKG on 2 beacons: `default` (5s period) and
  `test_beacon` (60s period).
* `reshareBeacon.sh` - Runs a reshare on the `default` and `test_beacon` beacons.
   Please read the comment at the top of the file for preparation.
* `testClient.sh` - Exercises the drand client against all 5 nodes.
* `testRelay.sh` - Starts a relay in each of the 5 Docker containers and exercises
   them.

## Preparation

### Quick run

You can quickly set up everything using `start.sh`.
This will build the Docker image, then launch `docker-compose`.
Finally, it will also start `utils/startBeacon.sh`, you can read more about this below.

The data directories will be written by the `drand` user inside the container.
If you'd like to view their contents, run the `./user.sh` script in this directory.

To stop the test instances, run `stop.sh`.

If you want to do run these scripts manually, then continue reading.

### Building the images

To run the docker-compose file, you will need to build `drand` and `drand-dev`
images. To do this, run the following command in the root directory of this repository:

```bash
make build_docker_all
```

You will have to do this each time that you change the source code.

### Creating the data directories

The first time you run the `docker-compose` file, you will also need to create the
directories corresponding to each container's data volumes and change their permissions
accordingly, otherwise the drand daemons will not be able to create the `.drand`
directory:

```bash
mkdir data_{0,1,2,3,4}
chmod 777 data_{0,1,2,3,4}
```
