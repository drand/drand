# relay-s3

A drand relay that writes randomness rounds to an AWS S3 bucket.

## Usage

```console
drand-relay-s3 run [arguments...]
```

Note: at minimum you'll need to specify a S3 bucket name and either a HTTP or GRPC drand endpoint to relay from.

### Example

```console
drand-relay-s3 run -chain 138a324aa6540f93d0dad002aa89454b1bec2b6e948682cde6bd4db40f4b7c9b -u http://pl-us.testnet.drand.sh -b drand-testnet -r eu-west-2
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
