#!/bin/bash

set -x

curr=$(pwd)
cd $GOPATH/src/github.com/dedis/drand

echo "[+] building drand ..."
go build

cp drand $curr/drand
chmod +x $curr/drand
cd $curr

tmp=$(mktemp -d)
echo "[+] base tmp folder: $tmp"

f1=$tmp/d1
f2=$tmp/d2
f3=$tmp/d3
f4=$tmp/d4
f5=$tmp/d5
a1="127.0.0.1:5000"
a2="127.0.0.1:6000"
a3="127.0.0.1:7000"
a4="127.0.0.1:8000"
a5="127.0.0.1:9000"
p1=5001
p2=6001
p3=7001
p4=8001
p5=9001

mkdir -p f1 f2 f3

echo "[+] generating keys"
./drand -f $f1 generate-keypair $a1 --tls-disable
./drand -f $f2 generate-keypair $a2 --tls-disable
./drand -f $f3 generate-keypair $a3 --tls-disable
./drand -f $f4 generate-keypair $a4 --tls-disable
./drand -f $f5 generate-keypair $a5 --tls-disable

echo "[+] running drand daemons..."
./drand -f $f1 --verbose 2 start --tls-disable --control $p1 & # > $tmp/log1 2>&1 &
./drand -f $f2 --verbose 2 start --tls-disable --control $p2 & # > $tmp/log2 2>&1 &
./drand -f $f3 --verbose 2 start --tls-disable --control $p3 & # > $tmp/log3 2>&1 &
./drand -f $f4 --verbose 2 start --tls-disable --control $p4 & # > $tmp/log4 2>&1 &
./drand -f $f5 --verbose 2 start --tls-disable --control $p5 & # > $tmp/log5 2>&1 &

sleep 0.1

echo "[+] creating group.toml file"
group=$tmp/group.toml
./drand group $f1/key/drand_id.public $f2/key/drand_id.public \
                $f3/key/drand_id.public $f4/key/drand_id.public \
                $f5/key/drand_id.public --out $group

echo "[+] launching dkg ..."
./drand -f $f1 share $group --control $p1 &
./drand -f $f2 share $group --control $p2 &
./drand -f $f3 share $group --control $p3 &
./drand -f $f4 share $group --control $p4 &
./drand -f $f5 share $group --control $p5 --leader

echo "[+] done"
