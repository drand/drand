.PHONY: test test-unit test-integration demo deploy-local linter install build client drand relay-http relay-gossip relay-s3

# Version values
MAYOR=0
MINOR=1
PATCH=0

VER_PACKAGE=github.com/drand/drand/common
CLI_PACKAGE=github.com/drand/drand/cmd/drand-cli

GIT_REVISION := $(shell git rev-parse HEAD)
BUILD_DATE := $(shell date -u +%d/%m/%Y@%H:%M:%S)

drand: build

####################  Lint and fmt process ##################

install_lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin v1.41.1

lint:
	golangci-lint --version
	golangci-lint run -E gofmt -E gosec -E goconst -E gocritic --timeout 5m

lint-todo:
	golangci-lint run -E stylecheck -E gosec -E goconst -E godox -E gocritic

fmt:
	@echo "Checking (& upgrading) formatting of files. (if this fail, re-run until success)"
	@{ \
		files=$$( go fmt ./... ); \
		if [ -n "$$files" ]; then \
		echo "Files not properly formatted: $$files"; \
		exit 1; \
		fi; \
	}

check-modtidy:
	go mod tidy
	git diff --exit-code -- go.mod go.sum

clean:
	go clean

############################################ Test ############################################

test: test-unit test-integration

test-unit:
	GO111MODULE=on go test -race -short -v ./...

test-unit-cover:
	GO111MODULE=on go test -short -v -coverprofile=coverage.txt -covermode=count -coverpkg=all $(go list ./... | grep -v /demo/)

test-integration:
	go test -v ./demo
	cd demo && go build && ./demo -build -test -debug

coverage:
	go get -u github.com/ory/go-acc
	go get -v -t -d ./...
	COVERAGE=true go-acc ./...

demo:
	cd demo && go build && ./demo -build
	#cd demo && sudo ./run.sh

############################################ Build ############################################

build_proto:
	go get -u github.com/golang/protobuf/protoc-gen-go@v1.5.2
	go get -u google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.1.0
	cd protobuf && sh ./compile_proto.sh

# create the "drand" binary and install it in $GOBIN
	go install -ldflags "-X $(VER_PACKAGE).MAYOR=$(MAYOR) -X $(VER_PACKAGE).MINOR=$(MINOR) -X $(VER_PACKAGE).PATCH=$(PATCH) -X $(CLI_PACKAGE).buildDate=$(BUILD_DATE) -X $(CLI_PACKAGE).gitCommit=$(GIT_REVISION)"

# create the "drand" binary in the current folder
	go build -o drand -mod=readonly -ldflags "-X $(VER_PACKAGE).MAYOR=$(MAYOR) -X $(VER_PACKAGE).MINOR=$(MINOR) -X $(VER_PACKAGE).PATCH=$(PATCH) -X $(CLI_PACKAGE).buildDate=$(BUILD_DATE) -X $(CLI_PACKAGE).gitCommit=$(GIT_REVISION)"

# create the "drand-client" binary in the current folder
	go build -o drand-client -mod=readonly -ldflags "-X $(VER_PACKAGE).MAYOR=$(MAYOR) -X $(VER_PACKAGE).MINOR=$(MINOR) -X $(VER_PACKAGE).PATCH=$(PATCH) -X main.buildDate=$(BUILD_DATE) -X main.gitCommit=$(GIT_REVISION)" ./cmd/client
drand-client: client

# create the "drand-relay-http" binary in the current folder
	go build -o drand-relay-http -mod=readonly -ldflags "-X $(VER_PACKAGE).MAYOR=$(MAYOR) -X $(VER_PACKAGE).MINOR=$(MINOR) -X $(VER_PACKAGE).PATCH=$(PATCH) -X main.buildDate=$(BUILD_DATE) -X main.gitCommit=$(GIT_REVISION)" ./cmd/relay

drand-relay-http: relay-http

# create the "drand-relay-gossip" binary in the current folder
	go build -o drand-relay-gossip -mod=readonly -ldflags "-X $(VER_PACKAGE).MAYOR=$(MAYOR) -X $(VER_PACKAGE).MINOR=$(MINOR) -X $(VER_PACKAGE).PATCH=$(PATCH) -X main.buildDate=$(BUILD_DATE) -X main.gitCommit=$(GIT_REVISION)" ./cmd/relay-gossip

drand-relay-gossip: relay-gossip

# create the "drand-relay-s3" binary in the current folder
	go build -o drand-relay-s3 -mod=readonly -ldflags "-X $(VER_PACKAGE).MAYOR=$(MAYOR) -X $(VER_PACKAGE).MINOR=$(MINOR) -X $(VER_PACKAGE).PATCH=$(PATCH) -X main.buildDate=$(BUILD_DATE) -X main.gitCommit=$(GIT_REVISION)" ./cmd/relay-s3

drand-relay-s3: relay-s3

build_all: drand drand-client drand-relay-http drand-relay-gossip drand-relay-s3
build_docker:
	docker build --build-arg version=`git describe --tags` --build-arg gitCommit=`git rev-parse HEAD` -t drandorg/go-drand:latest .

############################################ Deps ############################################

install_deps_linux:
	PROTOC_ZIP=protoc-3.14.0-linux-x86_64.zip
	curl -OL https://github.com/protocolbuffers/protobuf/releases/download/v3.14.0/$PROTOC_ZIP
	sudo unzip -o $PROTOC_ZIP -d /usr/local bin/protoc
	sudo unzip -o $PROTOC_ZIP -d /usr/local 'include/*'
	rm -f $PROTOC_ZIP

install_deps_darwin:
	brew install protobuf