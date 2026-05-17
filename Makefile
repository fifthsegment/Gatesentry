.PHONY: test test-go test-python build run clean-test install-test-deps coverage coverage-int

PYTHON ?= python3
PIP ?= pip3
GS_DNS_PORT ?= 10053
COVDIR ?= /tmp/gatesentry-covdata
COV_BIN ?= /tmp/gatesentry-cov-bin

clean-test:
	@echo "Cleaning up test artifacts..."
	@-kill `cat /tmp/gatesentry.pid 2>/dev/null` 2>/dev/null || true
	@-rm -f /tmp/gatesentry.pid /tmp/gatesentry.log
	@-rm -rf /tmp/gatesentry $(COVDIR)

install-test-deps:
	@echo "Installing Python test dependencies..."
	$(PIP) install --break-system-packages pytest dnspython requests 2>/dev/null || \
	$(PIP) install pytest dnspython requests 2>/dev/null || \
	echo "WARNING: pip install failed, Python tests may skip"

build: clean-test
	go build -o /tmp/gatesentry-bin .

run: build
	cd /tmp && ./gatesentry-bin

# ── Unit test coverage (fast, no server needed) ───────────────────────────
coverage:
	@echo "Collecting unit-test coverage..."
	@go test -coverprofile=coverage.txt -covermode=atomic ./application/... ./gatesentryproxy/... 2>/dev/null; \
	echo "Coverage saved to coverage.txt"

# ── Integration-test coverage (spins up instrumented server) ─────────────
coverage-int: install-test-deps
	@echo "Building coverage-instrumented binary..."
	go build -cover -covermode=atomic -coverpkg=./application/...,./gatesentryproxy/... -o $(COV_BIN) .
	@mkdir -p $(COVDIR)
	@echo "Starting instrumented server..."
	@kill `cat /tmp/gatesentry.pid 2>/dev/null` 2>/dev/null || true
	@rm -f /tmp/gatesentry.pid
	@GATESENTRY_DNS_PORT=$(GS_DNS_PORT) GOCOVERDIR=$(COVDIR) $(COV_BIN) > /tmp/gatesentry.log 2>&1 & echo $$! > /tmp/gatesentry.pid
	@echo "Waiting for server..."
	@for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do \
		if curl -s http://localhost:10786 > /dev/null 2>&1; then \
			echo "Server ready!"; break; \
		fi; \
		[ $$i -eq 15 ] && echo "FAILED" && cat /tmp/gatesentry.log && exit 1; \
		echo "  ... ($$i/15)"; sleep 2; \
	done
	@echo "Running integration tests against instrumented binary..."
	@GODEBUG=gotestcache=off go test -v -timeout 5m ./tests/...; \
	GO_RESULT=$$?; \
	echo ""; \
	echo "Running Python integration tests..."; \
	$(PYTHON) -m pytest tests/integration_test.py -v --tb=short --color=yes; \
	PY_RESULT=$$?; \
	echo "Shutting down server to flush coverage data..."; \
	kill `cat /tmp/gatesentry.pid` 2>/dev/null || true; \
	sleep 2; \
	echo "Converting coverage to profile..."; \
	go tool covdata textfmt -i=$(COVDIR) -o=intcoverage.txt 2>/dev/null; \
	echo "Merging unit + integration coverage..."; \
	grep -v "^mode:" intcoverage.txt >> coverage.txt 2>/dev/null || true; \
	rm -f /tmp/gatesentry.pid intcoverage.txt; \
	if [ $$GO_RESULT -ne 0 ] || [ $$PY_RESULT -ne 0 ]; then echo "Tests failed"; exit 1; fi

# ── Server-start helpers ─────────────────────────────────────────────────
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

# ── Test targets ─────────────────────────────────────────────────────────
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

# Full test: unit coverage + instrumented integration coverage
test: coverage-int
	@echo ""
	@echo "Total coverage:"
	@go tool cover -func=coverage.txt | tail -1
