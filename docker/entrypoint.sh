#!bin/sh

set -e
user=drand

if [ -n "$DOCKER_DEBUG" ]; then
   set -x
fi

if [ `id -u` -eq 0 ]; then
    echo "Changing user to $user"
    # ensure directories are writable
    su-exec "$user" test -w "${DRAND_HOME}" || chown -R -- "$user" "${DRAND_HOME}"
    exec su-exec "$user" "$0" $@
fi

if [ ! -d "${DRAND_HOME}/.drand" -a -n "${DRAND_PUBLIC_ADDRESS}" ]; then
    drand generate-keypair --tls-disable "${DRAND_PUBLIC_ADDRESS}"
fi

exec drand $@
