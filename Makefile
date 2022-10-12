.PHONY: test test-unit test-integration demo deploy-local linter install build client drand relay-http relay-gossip relay-s3

VER_PACKAGE=github.com/drand/drand/common
CLI_PACKAGE=github.com/drand/drand/cmd/drand-cli

GIT_REVISION := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u +%d/%m/%Y@%H:%M:%S)

drand: build

####################  Lint and fmt process ##################

install_lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.50

lint:
	golangci-lint --version
	golangci-lint run --timeout 5m

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
	go install github.com/ory/go-acc@latest
	go get -v -t -d ./...
	COVERAGE=true go-acc ./...

demo:
	cd demo && go build && ./demo -build
	#cd demo && sudo ./run.sh

############################################ Build ############################################

build_proto:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28.1
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2.0
	cd protobuf && sh ./compile_proto.sh

# create the "drand" binary and install it in $GOBIN
install:
	go install -ldflags "-X $(VER_PACKAGE).COMMIT=$(GIT_REVISION) -X $(VER_PACKAGE).BUILDDATE=$(BUILD_DATE) -X $(CLI_PACKAGE).buildDate=$(BUILD_DATE) -X $(CLI_PACKAGE).gitCommit=$(GIT_REVISION)"

# create the "drand" binary in the current folder
build:
	go build -o drand -mod=readonly -ldflags "-X $(VER_PACKAGE).COMMIT=$(GIT_REVISION) -X $(VER_PACKAGE).BUILDDATE=$(BUILD_DATE) -X $(CLI_PACKAGE).buildDate=$(BUILD_DATE) -X $(CLI_PACKAGE).gitCommit=$(GIT_REVISION)"

# create the "drand-client" binary in the current folder
client:
	go build -o drand-client -mod=readonly -ldflags "-X $(VER_PACKAGE).COMMIT=$(GIT_REVISION) -X $(VER_PACKAGE).BUILDDATE=$(BUILD_DATE) -X main.buildDate=$(BUILD_DATE) -X main.gitCommit=$(GIT_REVISION)" ./cmd/client
drand-client: client

# create the "drand-relay-http" binary in the current folder
relay-http:
	go build -o drand-relay-http -mod=readonly -ldflags "-X $(VER_PACKAGE).COMMIT=$(GIT_REVISION) -X $(VER_PACKAGE).BUILDDATE=$(BUILD_DATE) -X main.buildDate=$(BUILD_DATE) -X main.gitCommit=$(GIT_REVISION)" ./cmd/relay
drand-relay-http: relay-http

# create the "drand-relay-gossip" binary in the current folder
relay-gossip:
	go build -o drand-relay-gossip -mod=readonly -ldflags "-X $(VER_PACKAGE).COMMIT=$(GIT_REVISION) -X $(VER_PACKAGE).BUILDDATE=$(BUILD_DATE) -X main.buildDate=$(BUILD_DATE) -X main.gitCommit=$(GIT_REVISION)" ./cmd/relay-gossip
drand-relay-gossip: relay-gossip

# create the "drand-relay-s3" binary in the current folder
relay-s3:
	go build -o drand-relay-s3 -mod=readonly -ldflags "-X $(VER_PACKAGE).COMMIT=$(GIT_REVISION) -X $(VER_PACKAGE).BUILDDATE=$(BUILD_DATE) -X main.buildDate=$(BUILD_DATE) -X main.gitCommit=$(GIT_REVISION)" ./cmd/relay-s3
drand-relay-s3: relay-s3

build_all: drand drand-client drand-relay-http drand-relay-gossip drand-relay-s3

build_docker_all: build_docker build_docker_dev
build_docker:
	docker build --build-arg gitCommit=$(GIT_REVISION) --build-arg buildDate=$(BUILD_DATE) -t drandorg/go-drand:latest .

build_docker_dev:
	docker build -f test/docker/Dockerfile --build-arg gitCommit=$(GIT_REVISION) --build-arg buildDate=$(BUILD_DATE) -t drandorg/go-drand-dev:latest .
############################################ Deps ############################################

PROTOC_VERSION=3.19.6
PROTOC_ZIP_LINUX=protoc-$(PROTOC_VERSION)-linux-x86_64.zip
PROTOC_ZIP_DARWIN=protoc-$(PROTOC_VERSION)-osx-x86_64.zip
PROTOC_ZIP_DARWIN_M=protoc-$(PROTOC_VERSION)-osx-aarch_64.zip

install_deps_linux:
	curl -OL https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/$(PROTOC_ZIP_LINUX)
	echo Please provide your machine password to copy to /usr/local
	sudo unzip -o $(PROTOC_ZIP_LINUX) -d /usr/local bin/protoc 'include/*'
	sudo chmod a+x /usr/local/bin/protoc
	rm -f $(PROTOC_ZIP_LINUX)

install_deps_darwin:
	curl -OL https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/$(PROTOC_ZIP_DARWIN)
	echo Please provide your machine password to copy to /usr/local
	sudo unzip -o $(PROTOC_ZIP_DARWIN) -d /usr/local bin/protoc 'include/*'
	sudo chmod a+x /usr/local/bin/protoc
	rm -f $(PROTOC_ZIP_DARWIN)

install_deps_darwin-m:
	curl -OL https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/$(PROTOC_ZIP_DARWIN_M)
	echo Please provide your machine password to copy to /usr/local
	sudo unzip -o $(PROTOC_ZIP_DARWIN_M) -d /usr/local bin/protoc 'include/*'
	sudo chmod a+x /usr/local/bin/protoc
	rm -f $(PROTOC_ZIP_DARWIN_M)

