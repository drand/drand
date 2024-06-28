module github.com/drand/drand/internal/test-integration

go 1.19

require (
	github.com/drand/drand v1.4.7
	github.com/drand/kyber v1.2.0
)

require (
	github.com/BurntSushi/toml v1.2.1 // indirect
	github.com/drand/kyber-bls12381 v0.2.6 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/kilic/bls12-381 v0.1.0 // indirect
	github.com/nikkolasg/hexjson v0.1.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.24.0 // indirect
	golang.org/x/crypto v0.9.0 // indirect
	golang.org/x/net v0.10.0 // indirect
	golang.org/x/sys v0.8.0 // indirect
	golang.org/x/text v0.9.0 // indirect
	google.golang.org/genproto v0.0.0-20230410155749-daa745c078e1 // indirect
	google.golang.org/grpc v1.55.0 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
)

// This is is required to allow testing against the current code.
replace github.com/drand/drand => ../../../
