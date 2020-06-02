#!/bin/sh

# Use AMI: Amazon Linux AMI 2018.03.0 (HVM), SSD Volume Type - ami-01d025118d8e760db

# setup phase
sudo yum update -y
sudo yum install -y docker
sudo service docker start
sudo usermod -a -G docker ec2-user
docker login
docker pull drandorg/testnet-observer:1.0

# start observer client daemon
docker run -d \
        --restart always -p 42777:42777 -p 32111:32111 \
        -e DRAND_CLIENT_METRICS_ID=drand-testnet-observer-ohio \
        drandorg/testnet-observer:1.0

# start telegraf
docker run -d \
        --restart always \
        -v /home/ec2-user/telegraf.conf:/etc/telegraf/telegraf.conf:ro \
        telegraf --debug
