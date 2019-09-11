.PHONY: test all

test:
	@echo "second makefile: Path is $$PATH"
	PATH=$(PATH) ./docker_test.sh
	#cd test-integration-notls/ && ./test.sh
	#cd test-integration-tls/ && ./test.sh

all: test
