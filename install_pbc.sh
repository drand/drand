#!/bin/bash 

set -x

printh() {
    echo "$(hostname)\t: $1"
}

printh "PBC prescript installer"
sudo apt-get update
#printh " -> copying library"
#sudo cp "libbls384.so" "/usr/lib/"

sudo apt-get install -y wget
#STDLIB="libstd.deb"
#if [ ! -f "$STDLIB" ]; then 
    #wget "http://security.ubuntu.com/ubuntu/pool/main/g/gcc-4.8/libstdc++6_4.8.4-2ubuntu1~14.04.3_amd64.deb" -O "$STDLIB"
#fi
#dpkg -i "$STDLIB"

### installing libgmp anyway
sudo apt-get -y install libssl-dev
sudo apt-get -y install libgmp-dev
### doing symlinks for openssl weirdness ??
OLD="$(pwd)"
if [ ! -f "/lib/x86_64-linux-gnu/libcrypto.so.1.1" ]; then
    printh " -> linking libcrypto.so.1.1"
    cd "/usr/lib/x86_64-linux-gnu"
    sudo ln -s libcrypto.so libcrypto.so.1.1
    cd "$OLD"
fi

LINK="https://s3-us-west-2.amazonaws.com/dfinity/crypto/bn/latest/bn-latest-amd64-linux-ubuntu16.04.tar.gz"
TAR_NAME="bn-latest-amd64-linux-ubuntu16.04.tar.gz"
TAR_LIB_PATH="bn-r20170708-2-amd64-linux-ubuntu16.04/lib/libbls384.so"
SYS_LIB_PATH="/lib/libbls384.so"

extract() {
    echo "[+] Extracting the library."
    tar xvf "$TAR_NAME"
}

make_link() {
    echo "[+] Creating symlink"
    # simply do a symbolic link        
    sudo ln -s "$(pwd)/$TAR_LIB_PATH" "$SYS_LIB_PATH"
    sudo ldconfig
}

# check if library is already setup
if [ -f "$SYS_LIB_PATH" ]; then
    echo "[+] Library already installed. Exiting."
    sudo ldconfig
    exit 0
fi

# check if library is not extracted yet
if [ -f "$TAR_NAME" ] && [ ! -f "$TAR_LIB_PATH"]; then
    echo "[+] Library not extracted yet"
    extract
    make_link
    exit 0
fi 

# check if library is already downloaded and extracted
if [ -f "$TAR_LIB_PATH" ]; then
    echo "[+] Library already downloaded."
    make_link
    exit 0
fi


echo "[+] Downloading and extracting the library..."
# dl and extract the library
wget "$LINK"
extract
make_link
sudo ldconfig
echo " == ldconfigs ALL"
sudo ldconfig -v
echo " == ldconfig BLS"
sudo ldconfig -v | grep -i bls
echo "[+] Bye !"
exit 0
