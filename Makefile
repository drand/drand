.PHONY: test test-unit test-integration demo deploy-local linter

test: linter test-unit test-integration

test-unit:
	go test -v -mod=vendor ./...

test-integration:
	$(MAKE) -C test-integration test

linter:
	@echo "Checking (& upgrading) correct formatting of files... (if this fail, re-run until success)"
	@{ \
		files=$$( go fmt ./... ); \
		if [ -n "$$files" ]; then \
		echo "Files not properly formatted: $$files"; \
		exit 1; \
		fi; \
	}

demo:
deploy-local:
	cd demo && ./run.sh