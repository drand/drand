#!/bin/sh

# setup phase
sudo yum update -y
sudo yum install -y docker
sudo service docker start
sudo usermod -a -G docker ec2-user
docker login #XXX
docker pull drandorg/testnet-observer:1.0

# start daemon
docker run -d \
        --restart always -p 42777:42777 -p 32111:32111 \
        -e DRAND_CLIENT_METRICS_ID=drand-testnet-observer-ohio \
        drandorg/testnet-observer:1.0
