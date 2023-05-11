#!/usr/bin/env bash

echo "This script will create the drand group and user in your system."
echo "Press [Enter] to continue"
read -r

sudo addgroup -gid 994 drand
sudo adduser --system --no-create-home -uid 996 -gid 994 drand

echo "Now, your user will be added to the drand group"
echo "Press [Enter] to continue"
read -r

sudo usermod -a -G drand "${USER}"
