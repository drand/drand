#!/bin/bash 

set -x

printh() {
    echo "$(hostname)\t: $1"
}

printh "PBC prescript installer"
sudo cat /etc/os-release
sudo apt-get update
sudo apt-get -y install libssl-dev \
                        libgmp-dev \
                        git \
                        make

sudo git clone https://github.com/dfinity/bn /bn
sudo wget -O - http://llvm.org/apt/llvm-snapshot.gpg.key|sudo apt-key add - 
echo "deb http://llvm.org/apt/trusty/ llvm-toolchain-trusty-3.8 main" | sudo tee /etc/apt/sources.list
sudo apt-get update
sudo apt-get -y install llvm-3.8 clang-3.8 g++
export CXX=clang++-3.8
export PATH=$PATH:/usr/lib/llvm-3.8/bin/
export LD_LIBRARY_PATH=/lib:/usr/lib:/usr/local/lib
cd /bn
make
sudo make install
