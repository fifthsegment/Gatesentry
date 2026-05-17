.PHONY: test test-go test-python build run clean-test install-test-deps coverage

PYTHON ?= python3
PIP ?= pip3
GS_DNS_PORT ?= 10053

clean-test:
	@echo "Cleaning up test artifacts..."
	@-kill `cat /tmp/gatesentry.pid 2>/dev/null` 2>/dev/null || true
	@-rm -f /tmp/gatesentry.pid /tmp/gatesentry.log
	@-rm -rf /tmp/gatesentry

install-test-deps:
	@echo "Installing Python test dependencies..."
	$(PIP) install --break-system-packages pytest dnspython requests 2>/dev/null || \
	$(PIP) install pytest dnspython requests 2>/dev/null || \
	echo "WARNING: pip install failed, Python tests may skip"

build: clean-test
	go build -o /tmp/gatesentry-bin .

run: build
	cd /tmp && ./gatesentry-bin

_start-server: clean-test build
	@echo "Starting Gatesentry server in background (DNS on port $(GS_DNS_PORT))..."
	@cd /tmp && GATESENTRY_DNS_PORT=$(GS_DNS_PORT) ./gatesentry-bin > /tmp/gatesentry.log 2>&1 & echo $$! > /tmp/gatesentry.pid
	@echo "Waiting for server to be ready..."
	@for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do \
		if curl -s http://localhost:10786 > /dev/null 2>&1; then \
			echo "Server is ready!"; \
			break; \
		fi; \
		if [ $$i -eq 15 ]; then \
			echo "Server failed to start. Logs:"; \
			cat /tmp/gatesentry.log; \
			kill `cat /tmp/gatesentry.pid` 2>/dev/null || true; \
			rm -f /tmp/gatesentry.pid; \
			exit 1; \
		fi; \
		echo "Waiting... ($$i/15)"; \
		sleep 2; \
	done

_stop-server:
	@echo "Stopping server..."
	@kill `cat /tmp/gatesentry.pid` 2>/dev/null || true
	@rm -f /tmp/gatesentry.pid

coverage:
	@echo "Collecting Go coverage across all packages..."
	@go test -coverprofile=coverage.txt -covermode=atomic ./application/... ./gatesentryproxy/... 2>/dev/null; \
	echo "Coverage collected."

test-go: _start-server
	@echo "Running Go integration tests..."
	@GODEBUG=gotestcache=off go test -v -timeout 5m ./tests/...; \
	GO_RESULT=$$?; \
	$(MAKE) _stop-server; \
	if [ $$GO_RESULT -ne 0 ]; then echo "Go tests failed"; exit 1; fi

test-python: install-test-deps _start-server
	@echo "Running Python integration tests..."
	@$(PYTHON) -m pytest tests/integration_test.py -v --tb=short --color=yes; \
	PY_RESULT=$$?; \
	$(MAKE) _stop-server; \
	if [ $$PY_RESULT -ne 0 ]; then echo "Python tests failed"; exit 1; fi

# Run all integration tests with server + collect Go coverage
test: coverage install-test-deps _start-server
	@echo "Running Go integration tests..."
	@GODEBUG=gotestcache=off go test -v -timeout 5m ./tests/...; \
	GO_RESULT=$$?; \
	if [ $$GO_RESULT -ne 0 ]; then \
		echo "Go tests failed — aborting"; \
		$(MAKE) _stop-server; \
		exit 1; \
	fi
	@echo ""
	@echo "Running Python integration tests..."
	@$(PYTHON) -m pytest tests/integration_test.py -v --tb=short --color=yes; \
	PY_RESULT=$$?; \
	$(MAKE) _stop-server; \
	if [ $$PY_RESULT -ne 0 ]; then echo "Python tests failed"; exit 1; fi
