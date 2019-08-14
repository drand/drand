.PHONY: test all

test:
	./docker_test.sh
	cd test-integration-notls/ && ./test.sh
	cd test-integration-tls/ && ./test.sh

all: test
