#!/bin/sh
## this file is used by the demo projects and CLI for building docker images

set -e
user=drand
expected_binary_dir="/usr/local/bin/"
binary=drand

if [ -n "$DOCKER_DEBUG" ]; then
   set -x
fi

# Check we're the right user, if we're not, change user and re-exec this script
if [ `id -u` -eq 0 ]; then
    echo "Changing user to $user"
    # ensure directories are writable
    su-exec "$user" test -w "${DRAND_HOME}" || chown -R -- "$user" "${DRAND_HOME}"
    exec su-exec "$user" "$0" $@
fi

if [ ! -z $1 ]; then
  binary=$1
  binary_path=${expected_binary_dir}${binary}
  if [ ! -f "${binary_path}" ]; then
    echo "Specified binary ${binary} not found in $expected_binary_dir"
    exit 1
  fi
  shift
fi

if [ ${binary} = "drand" -a ! -d "${DRAND_HOME}/.drand" -a -n "${DRAND_PUBLIC_ADDRESS}" ]; then
    drand generate-keypair --tls-disable "${DRAND_PUBLIC_ADDRESS}"
fi

exec ${binary} $@
