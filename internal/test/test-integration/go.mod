module github.com/drand/drand/internal/test-integration

go 1.20

require (
	github.com/drand/drand v1.4.7
	github.com/drand/kyber v1.2.0
)

require (
	github.com/BurntSushi/toml v1.3.2 // indirect
	github.com/drand/kyber-bls12381 v0.3.1 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/kilic/bls12-381 v0.1.0 // indirect
	github.com/nikkolasg/hexjson v0.1.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.24.0 // indirect
	golang.org/x/crypto v0.10.0 // indirect
	golang.org/x/net v0.11.0 // indirect
	golang.org/x/sys v0.10.0 // indirect
	golang.org/x/text v0.11.0 // indirect
	google.golang.org/genproto v0.0.0-20230629202037-9506855d4529 // indirect
	google.golang.org/grpc v1.56.1 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
)

// This is is required to allow testing against the current code.
replace github.com/drand/drand => ../../../
