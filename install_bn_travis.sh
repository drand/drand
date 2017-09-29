#!/bin/bash 

# This script installs the bn library on the Travis running machine

printh() {
    echo "$(hostname)\t: $1"
}

printh "PBC prescript installer"
sudo cat /etc/os-release
sudo rm -rf /usr/local/clang-3.4
sudo add-apt-repository -y ppa:ubuntu-toolchain-r/test
sudo add-apt-repository 'deb http://llvm.org/apt/trusty/ llvm-toolchain-trusty-3.8 main'
sudo apt-get update
sudo apt-get -y install libssl-dev \
                        libgmp-dev \
                        git \
                        make
#sudo wget -O - http://llvm.org/apt/llvm-snapshot.gpg.key|sudo apt-key add - 
#echo "deb http://llvm.org/apt/trusty/ llvm-toolchain-trusty-3.8 main" | sudo tee /etc/apt/sources.list
sudo apt-get install --force-yes llvm-3.8 llvm-3.8-dev clang-3.8 clang-3.8-dev libc++-dev
export CXX="clang++-3.8"
export PATH="$PATH:/usr/lib/llvm-3.8/bin/"
echo "/usr/local/lib" | sudo tee /etc/ld.so.conf
#sudo ln -sf /usr/bin/clang-3.8 /usr/bin/clang
#sudo ln -sf /usr/bin/llc-3.8 /usr/bin/llc
#sudo ln -sf /usr/bin/opt-3.8 /usr/bin/opt

LIBPATH="$HOME/bn"
git clone https://github.com/dfinity/bn "$LIBPATH"
cd "$LIBPATH"
make
sudo make install
sudo ldconfig
