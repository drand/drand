.PHONY: test test-unit test-integration demo deploy-local linter install build

test: test-unit test-integration

test-unit:
	GO111MODULE=on go test -race -v ./...

test-unit-cover:
	GO111MODULE=on go test -v -coverprofile=coverage.txt -covermode=atomic -coverpkg=all ./...

test-integration:
	cd demo && go build && ./demo -test -binary "" -nocurl && ./demo -build -test -debug

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
	GO111MODULE=on go install -ldflags "-X main.version=`git describe --tags` -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`" 

# create the "drand" binary in the current folder
build: 
	go build -ldflags "-X main.version=`git describe --tags` -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`" 
