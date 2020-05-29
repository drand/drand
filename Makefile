.PHONY: test test-unit test-integration demo deploy-local linter install build

test: test-unit test-integration

test-unit:
	GO111MODULE=on go test -race -v ./...

test-unit-cover:
	GO111MODULE=on go test -v -coverprofile=coverage.txt -covermode=count -coverpkg=all $(go list ./... | grep -v /demo/)

test-integration:
	go test -v ./demo
	cd demo && go build && ./demo -build -test -debug

linter:
	@echo "Checking (& upgrading) formatting of files. (if this fail, re-run until success)"
	@{ \
		files=$$( go fmt ./... ); \
		if [ -n "$$files" ]; then \
		echo "Files not properly formatted: $$files"; \
		exit 1; \
		fi; \
	}

demo:
	cd demo && go build && ./demo -build
	#cd demo && sudo ./run.sh

# create the "drand" binary and install it in $GOBIN
install:
	go install -ldflags "-X github.com/drand/drand/cmd/drand-cli.version=`git describe --tags` -X github.com/drand/drand/cmd/drand-cli.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X github.com/drand/drand/cmd/drand-cli.gitCommit=`git rev-parse HEAD`"

# create the "drand" binary in the current folder
build:
	go build -o drand -mod=readonly -ldflags "-X github.com/drand/drand/cmd/drand-cli.version=`git describe --tags` -X github.com/drand/drand/cmd/drand-cli.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X github.com/drand/drand/cmd/drand-cli.gitCommit=`git rev-parse HEAD`"

drand: build

# create the "drand-client" binary in the current folder
client:
	go build -o drand-client -mod=readonly -ldflags "-X github.com/drand/drand/cmd/demo-client.version=`git describe --tags` -X github.com/drand/cmd/demo-client.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X github.com/drand/drand/cmd/demo-client.gitCommit=`git rev-parse HEAD`" ./cmd/demo-client
drand-client: client

# create the "drand-relay-http" binary in the current folder
relay-http:
	go build -o drand-relay-http -mod=readonly -ldflags "-X github.com/drand/drand/cmd/relay.version=`git describe --tags` -X github.com/drand/cmd/relay.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X github.com/drand/drand/cmd/relay.gitCommit=`git rev-parse HEAD`" ./cmd/relay
drand-relay-http: relay-http

# create the "drand-relay-gossip" binary in the current folder
relay-gossip:
	go build -o drand-relay-gossip -mod=readonly -ldflags "-X github.com/drand/drand/cmd/relay-gossip.version=`git describe --tags` -X github.com/drand/drand/cmd/relay-gossip.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X github.com/drand/drand/cmd/relay-gossip.gitCommit=`git rev-parse HEAD`" ./cmd/relay-gossip
drand-relay-gossip: relay-gossip
