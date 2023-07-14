module github.com/drand/drand/internal/test-integration

go 1.20

require (
	github.com/drand/drand v1.4.7
	github.com/drand/kyber v1.1.18
)

require (
	github.com/BurntSushi/toml v1.2.1 // indirect
	github.com/drand/kyber-bls12381 v0.2.5 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/kilic/bls12-381 v0.1.0 // indirect
	github.com/nikkolasg/hexjson v0.1.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.9.0 // indirect
	go.uber.org/zap v1.24.0 // indirect
	golang.org/x/crypto v0.6.0 // indirect
	golang.org/x/net v0.7.0 // indirect
	golang.org/x/sys v0.5.0 // indirect
	golang.org/x/text v0.7.0 // indirect
	google.golang.org/genproto v0.0.0-20230227214838-9b19f0bdc514 // indirect
	google.golang.org/grpc v1.53.0 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
)

// This is is required to allow testing against the current code.
replace github.com/drand/drand => ../../../
