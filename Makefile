.PHONY: install build

# create the "drand" binary and install it in $GOBIN
install:
	GO111MODULE=on go install -mod=vendor -ldflags "-X main.version=`git describe --tags` -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`" 

# create the "drand" binary in the current folder
build: 
	go build -ldflags "-X main.version=`git describe --tags` -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`" 