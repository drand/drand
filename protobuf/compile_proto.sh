#!/usr/bin/env bash

# fail automatically as soon as an error is detected
set -e

go generate
echo
echo "Done!"
