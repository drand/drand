# Client observatory deployment

## Prepare client image

Build the observer client docker image:

        cd cmd/client/testnet-observer
        docker build -t drandorg/testnet-observer:1.0 -f ./Dockerfile ../../..

Upload client image to Docker Hub:

        docker push drandorg/testnet-observer:1.0

## Deploy client

Repeat the steps below for each client deployment.

Provision an EC2 instance based on `Amazon Linux AMI 2018.03.0 (HVM), SSD Volume Type - ami-01d025118d8e760db`.

Log into the instance. Install required software:

        sudo yum update -y
        sudo yum install -y docker
        sudo service docker start
        sudo usermod -a -G docker ec2-user

Log out and back into the instance.

        docker login
        docker pull drandorg/testnet-observer:1.0

Start the observing client, where `$REGION` is the name of its geographic location:

        docker run -d \
          --restart always \
          -p 42777:42777 -p 32111:32111 \
          -e DRAND_CLIENT_METRICS_ID=drand-testnet-observer-$REGION \
          drandorg/testnet-observer:1.0

## Deploy metrics collector

After all clients have been deployed, start the Telegraf metric collector service, as shown here.

Provision an EC2 instance based on `Amazon Linux AMI 2018.03.0 (HVM), SSD Volume Type - ami-01d025118d8e760db`.

Log into the instance. Install required software:

        sudo yum update -y
        sudo yum install -y docker
        sudo service docker start
        sudo usermod -a -G docker ec2-user

Log out and back into the instance.

        docker login
        docker pull telegraf

Prepare the Telegraf service configuration:
- Use the template `cmd/client/testnet-observer/telegraf.conf` in the repo. 
- Substitute the InfluxDB address, token and bucket, marked as `$INFLUXDB_ADDRESS`, `$INFLUXDB_TOKEN` and `$INFLUXDB_BUCKET`, respectively.
- Substitute the Prometheus Input URLs to point to the list of clients you deployed in the previous step, at port 32111, at the location marked as `$CDN_URLS`.
- Copy the resulting `telegraf.conf` to the EC2 instance at `/home/ec2-user/telegraf.conf`.

Start the telegraf service:

        docker run -d \
                --restart always \
                -v /home/ec2-user/telegraf.conf:/etc/telegraf/telegraf.conf:ro \
                telegraf --debug

At this point, the observing network is fully deployed.

# Testnet-specific links

## Drand HTTP and gossip relay endpoints
The testnet network (including drand nodes, relays and CDNs) is described at https://github.com/drand/drand-infra#deployments.

Use this information to configure your client's `entrypoint.sh` to hit all HTTP CDNs and relays.

Relays are listed with their DNS addresses. Use `dig` to recover their P2P addresses, e.g.

        dig _dnsaddr.pl-us.testnet.drand.sh TXT

## InfluxDB
The testnet InfluxDB instance is at https://us-west-2-1.aws.cloud2.influxdata.com. The client observatory uses the bucket `observatory`.
