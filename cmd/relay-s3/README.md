# relay-s3

A drand relay that writes randomness rounds to an AWS S3 bucket.

## Usage

```sh
drand-relay-s3 run [arguments...]
```

Note: at minimum you'll need to specify a S3 bucket name and either a HTTP, gRPC or libp2p pubsub drand endpoint to relay from.

**Example**

```sh
drand-relay-s3 run -hash 138a324aa6540f93d0dad002aa89454b1bec2b6e948682cde6bd4db40f4b7c9b -url http://pl-us.testnet.drand.sh -bucket drand-testnet -region eu-west-2
```

### Sync bucket with randomness chain

The `sync` command will ensure the AWS S3 bucket is fully sync'd with the randomness chain. i.e. it ensures all randomness rounds to date (and generated during the sync) are uploaded to the S3 bucket. This may take a while, but if you need to stop you can start it again from a specific round number using the `-begin` flag.

```sh
drand-relay-s3 sync [arguments...]
```

## Prerequesites

### Credentials

Ensure AWS credentials file is in `~/.aws/credentials` - it should look like:

```
[default]
aws_access_key_id=AKIAIOSFODNN7EXAMPLE
aws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
```

### S3 bucket

Ensure you have an S3 bucket ready with _public access_ enabled. The credentials in `~/.aws/credentials` should have write access to it. Make sure the following CORS configuration is applied to allow web applications to request randomness:

```xml
<CORSConfiguration>
 <CORSRule>
   <AllowedOrigin>*</AllowedOrigin>
   <AllowedMethod>GET</AllowedMethod>
 </CORSRule>
</CORSConfiguration>
```
