#!/bin/bash

# Build Debian package using Docker on macOS
set -e

echo "Building Debian package for drand..."

# Build the package using Docker with newer Go version
docker run --rm \
  -v "$(pwd)":/workspace \
  -w /workspace \
  debian:bookworm-slim \
  bash -c "
    apt-get update && \
    apt-get install -y devscripts debhelper git ca-certificates curl && \
    # Install Go 1.23
    curl -L https://go.dev/dl/go1.23.4.linux-arm64.tar.gz -o /tmp/go.tar.gz && \
    tar -C /usr/local -xzf /tmp/go.tar.gz && \
    export PATH=/usr/local/go/bin:\$PATH && \
    export GOROOT=/usr/local/go && \
    export GOOS=linux && \
    export GOARCH=arm64 && \
    # Change GOARCH to amd64 for x86_64 Linux systems && \
    export CGO_ENABLED=0 && \
    dpkg-buildpackage -us -uc -b && \
    # Copy generated files to workspace root
    cp ../*.deb ../*.buildinfo ../*.changes /workspace/ 2>/dev/null || true
  "

echo "Package built successfully!"
echo "Generated files:"
ls -la *.deb *.buildinfo *.changes 2>/dev/null || echo "No package files found in current directory"
