module github.com/drand/drand/v2

go 1.20

require (
	github.com/BurntSushi/toml v1.3.2
	github.com/ardanlabs/darwin/v2 v2.0.0
	github.com/briandowns/spinner v1.23.0
	github.com/drand/kyber v1.2.0
	github.com/drand/kyber-bls12381 v0.3.1
	github.com/go-chi/chi/v5 v5.0.12
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/jedib0t/go-pretty/v6 v6.5.4
	github.com/jmoiron/sqlx v1.3.5
	github.com/jonboulle/clockwork v0.4.0
	github.com/lib/pq v1.10.9
	github.com/nikkolasg/hexjson v0.1.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.18.0
	github.com/prometheus/common v0.47.0
	github.com/rogpeppe/go-internal v1.12.0
	github.com/stretchr/testify v1.8.4
	github.com/urfave/cli/v2 v2.27.1
	go.etcd.io/bbolt v1.3.8
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.48.1-0.20240215192441-65f36677bddc
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.48.0
	go.opentelemetry.io/otel v1.23.1
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.23.1
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.23.1
	go.opentelemetry.io/otel/sdk v1.23.1
	go.opentelemetry.io/otel/trace v1.23.1
	go.uber.org/zap v1.26.0
	golang.org/x/crypto v0.19.0
	golang.org/x/net v0.21.0
	golang.org/x/sys v0.17.0
	google.golang.org/grpc v1.61.1
	google.golang.org/protobuf v1.32.0
)

//nolint:gomoddirectives
// Without this replace, urfave/cli will have race conditions in our tests
replace github.com/urfave/cli/v2 => github.com/urfave/cli/v2 v2.19.3

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.19.1 // indirect
	github.com/kilic/bls12-381 v0.1.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.6.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/stretchr/objx v0.5.1 // indirect
	github.com/xrash/smetrics v0.0.0-20231213231151-1d8dd44e695e // indirect
	go.opentelemetry.io/otel/metric v1.23.1 // indirect
	go.opentelemetry.io/proto/otlp v1.1.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/term v0.17.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240213162025-012b6fc9bca9 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240213162025-012b6fc9bca9 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
